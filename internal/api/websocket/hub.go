package websocket

import (
	"log"
	"sync"
	"time"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/api/dto"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/logging"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
)

// clientSendBuffer es cuántos eventos puede acumular un cliente antes de
// considerarse lento y ser desconectado. Un valor bajo detecta clientes
// atascados rápido sin arriesgar mucha memoria por conexión.
const clientSendBuffer = 16

// JobLister es lo mínimo que el hub necesita del supervisor: la lista de
// snapshots para construir el snapshot inicial y recalcular el resumen.
type JobLister interface {
	ListJobs() []supervisor.Snapshot
}

// Hub reparte eventos del supervisor entre todos los navegadores
// conectados a /ws. Un único goroutine (run) posee el mapa de clientes, así
// que register/unregister/broadcast nunca compiten por un mutex: se
// serializan pasando por channels.
type Hub struct {
	logger     *log.Logger
	register   chan *Client
	unregister chan *Client
	broadcast  chan Event
	done       chan struct{}
	closeOnce  sync.Once

	// lister se fija una sola vez durante el arranque (antes de que el hub
	// reciba tráfico real), por eso no necesita su propio mutex: ver la
	// nota en SetLister.
	lister JobLister

	// countReq es solo para pruebas: permite consultar cuántos clientes
	// hay registrados ahora mismo, pasando por el mismo goroutine que los
	// posee en vez de exponer el mapa directamente.
	countReq chan chan int
}

// NewHub crea el hub y arranca su goroutine de reparto. El lister puede
// fijarse después con SetLister si el supervisor todavía no existe en el
// momento de crear el hub (dependencia circular en el arranque: el
// supervisor necesita el publisher del hub, y el hub necesita poder listar
// los jobs del supervisor).
func NewHub(logger *log.Logger) *Hub {
	if logger == nil {
		logger = log.Default()
	}
	h := &Hub{
		logger:     logger,
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan Event, 64),
		done:       make(chan struct{}),
		countReq:   make(chan chan int),
	}
	go h.run()
	return h
}

// SetLister fija la fuente de snapshots. Debe llamarse durante el arranque,
// antes de sup.StartAll() y antes de aceptar conexiones en /ws: a partir de
// ahí distintas goroutines empiezan a leer h.lister concurrentemente.
func (h *Hub) SetLister(lister JobLister) { h.lister = lister }

func (h *Hub) run() {
	clients := make(map[*Client]struct{})
	defer func() {
		for c := range clients {
			c.evict(StatusGoingDown)
		}
	}()
	// run() vive en su propia goroutine desde NewHub; sin este recover un
	// panic aquí tumbaría todo el proceso en vez de quedar contenido.
	defer func() {
		if v := recover(); v != nil {
			h.logger.Printf("websocket: panic recuperado en el hub: %v", v)
		}
	}()
	for {
		select {
		case c := <-h.register:
			clients[c] = struct{}{}
		case c := <-h.unregister:
			if _, ok := clients[c]; ok {
				delete(clients, c)
				c.evict(StatusNormal)
			}
		case event := <-h.broadcast:
			for c := range clients {
				select {
				case c.send <- event:
				default:
					// Cliente lento: no bloqueamos al resto. Se
					// desconecta para no acumular eventos obsoletos
					// indefinidamente en su buffer.
					delete(clients, c)
					c.evict(StatusSlowClient)
					h.logger.Printf("websocket: cliente lento desconectado")
				}
			}
		case reply := <-h.countReq:
			reply <- len(clients)
		case <-h.done:
			return
		}
	}
}

// count es solo para pruebas.
func (h *Hub) count() int {
	reply := make(chan int, 1)
	select {
	case h.countReq <- reply:
	case <-h.done:
		return 0
	}
	select {
	case n := <-reply:
		return n
	case <-h.done:
		return 0
	}
}

// Broadcast encola un evento para todos los clientes conectados. No
// bloquea al llamador salvo que el búfer interno (64) esté lleno, y se
// vuelve un no-op silencioso si el hub ya se cerró.
func (h *Hub) Broadcast(event Event) {
	select {
	case h.broadcast <- event:
	case <-h.done:
	}
}

// PublishLog envía una línea recién escrita sin esperar al siguiente
// refresco HTTP de la consola.
func (h *Hub) PublishLog(job string, entry logging.Entry) {
	h.Broadcast(Event{Type: "log.entry", LogJob: job, LogEntry: &entry, Timestamp: time.Now()})
}

// PublishJobChange adapta un supervisor.Snapshot al Event correspondiente.
// Su forma (func(supervisor.Snapshot)) coincide con supervisor.Publisher a
// propósito: se pasa directamente como argumento a supervisor.New.
func (h *Hub) PublishJobChange(snap supervisor.Snapshot) {
	job := dto.FromSnapshot(snap)
	summary := h.summary()
	h.Broadcast(Event{Type: jobEventType(snap.State), Job: &job, Summary: &summary, Timestamp: time.Now()})
}

func (h *Hub) summary() dto.Summary {
	if h.lister == nil {
		return dto.Summary{}
	}
	return dto.SummaryFromSnapshots(h.lister.ListJobs())
}

// Snapshot construye el evento "jobs.snapshot" que se envía a cada cliente
// justo después de conectarse.
func (h *Hub) Snapshot() Event {
	var jobs []dto.JobResponse
	if h.lister != nil {
		snaps := h.lister.ListJobs()
		jobs = make([]dto.JobResponse, 0, len(snaps))
		for _, snap := range snaps {
			jobs = append(jobs, dto.FromSnapshot(snap))
		}
	}
	summary := h.summary()
	return Event{Type: "jobs.snapshot", Jobs: jobs, Summary: &summary, Timestamp: time.Now()}
}

// Register añade un cliente al hub. Devuelve false si el hub ya está
// cerrado, en cuyo caso el llamador debe cerrar la conexión sin servirla.
func (h *Hub) Register(c *Client) bool {
	select {
	case h.register <- c:
		return true
	case <-h.done:
		return false
	}
}

// Unregister quita un cliente. Es seguro llamarla más de una vez para el
// mismo cliente (por ejemplo si el hub ya se cerró mientras tanto).
func (h *Hub) Unregister(c *Client) {
	select {
	case h.unregister <- c:
	case <-h.done:
	}
}

// Close detiene el hub y cierra todas las conexiones activas. Es lo que
// permite que http.Server.Shutdown no se quede esperando indefinidamente a
// conexiones /ws ya "hijacked" (Shutdown no las cierra por sí solo).
// Seguro de llamar más de una vez.
func (h *Hub) Close() {
	h.closeOnce.Do(func() { close(h.done) })
}
