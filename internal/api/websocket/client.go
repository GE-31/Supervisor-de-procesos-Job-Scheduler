package websocket

import (
	"context"
	"log"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	// writeTimeout limita cuánto puede tardar un Write o un Ping antes de
	// darse por vencido con ese cliente.
	writeTimeout = 5 * time.Second
	// pingInterval mantiene viva la conexión y detecta clientes muertos
	// (proxies y navegadores pueden cerrar conexiones inactivas).
	pingInterval = 20 * time.Second
	// readLimit es generoso mas no ilimitado: no se aceptan comandos por
	// este canal, así que cualquier mensaje entrante se descarta, pero hay
	// que ponerle un techo para no dejar que un cliente hostil agote memoria.
	readLimit = 4096
)

// Códigos de cierre usados al desconectar un cliente desde el servidor.
const (
	StatusNormal     = coderws.StatusNormalClosure
	StatusGoingDown  = coderws.StatusServiceRestart
	StatusSlowClient = coderws.StatusPolicyViolation
)

// Client envuelve una conexión WebSocket ya aceptada. Tiene una única
// goroutine escritora (writeLoop): todos los eventos y pings salen por ahí,
// nunca concurrentemente, que es el único requisito real de coder/websocket
// para Write/Ping. readLoop existe solo para que la librería pueda procesar
// pings/pongs de control y para detectar cuándo el cliente se desconecta;
// cualquier mensaje que el navegador mande se descarta sin interpretarlo.
type Client struct {
	conn   *coderws.Conn
	send   chan Event
	hub    *Hub
	logger *log.Logger
}

func newClient(conn *coderws.Conn, hub *Hub, logger *log.Logger) *Client {
	return &Client{conn: conn, send: make(chan Event, clientSendBuffer), hub: hub, logger: logger}
}

// evict la llama el hub (desde su único goroutine run()) al quitar un
// cliente de su mapa, para cerrar también la conexión física. Sin esto,
// http.Server.Shutdown se quedaría esperando indefinidamente a que estas
// conexiones "hijacked" terminen por sí solas.
func (c *Client) evict(code coderws.StatusCode) {
	if c.conn != nil {
		_ = c.conn.Close(code, "")
	}
}

// serve bloquea hasta que la conexión termina (el cliente se desconecta,
// ocurre un error, o el hub cierra su canal send) y siempre desregistra al
// cliente al salir.
func (c *Client) serve(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer c.hub.Unregister(c)

	go c.readLoop(ctx, cancel)
	c.writeLoop(ctx)
}

func (c *Client) readLoop(ctx context.Context, cancel context.CancelFunc) {
	defer cancel()
	// Corre en su propia goroutine (no la de la solicitud HTTP), así que un
	// panic aquí no lo atraparía el middleware de recovery del servidor.
	defer func() {
		if v := recover(); v != nil {
			c.logger.Printf("websocket: panic recuperado en readLoop: %v", v)
		}
	}()
	c.conn.SetReadLimit(readLimit)
	for {
		if _, _, err := c.conn.Read(ctx); err != nil {
			return
		}
	}
}

func (c *Client) writeLoop(ctx context.Context) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-c.send:
			if !ok {
				return
			}
			if !c.write(ctx, event) {
				return
			}
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, writeTimeout)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (c *Client) write(ctx context.Context, event Event) bool {
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := wsjson.Write(writeCtx, c.conn, event); err != nil {
		return false
	}
	return true
}
