package websocket

import (
	"log"
	"net/http"

	coderws "github.com/coder/websocket"
)

// Handler expone GET /ws. No se fija OriginPatterns: dejar AcceptOptions
// con su valor por defecto hace que coder/websocket exija que el Origin
// coincida con el host de la propia solicitud (el mismo host que sirve el
// dashboard), que es exactamente la validación pedida — sin necesitar
// conocer de antemano el host o el puerto configurado.
func Handler(hub *Hub, logger *log.Logger) http.HandlerFunc {
	if logger == nil {
		logger = log.Default()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "método no permitido", http.StatusMethodNotAllowed)
			return
		}
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			logger.Printf("websocket: aceptar conexión: %v", err)
			return
		}
		client := newClient(conn, hub, logger)
		if !hub.Register(client) {
			_ = conn.Close(StatusGoingDown, "servidor cerrando")
			return
		}
		// Snapshot inicial inmediato: el navegador no depende del primer
		// ciclo de polling ni de esperar al próximo cambio de estado.
		select {
		case client.send <- hub.Snapshot():
		default:
		}
		client.serve(r.Context())
	}
}
