package server

import (
	"net/http"
)

// routes construye el ServeMux propio de orders-web. Solo existen rutas
// explícitas: no hay un proxy genérico capaz de reenviar a una URL arbitraria
// indicada por el cliente.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir(staticDir))))

	mux.HandleFunc("GET /proxy/health", s.proxyHealth)
	mux.HandleFunc("GET /proxy/orders/stats", s.proxyStats)
	mux.HandleFunc("GET /proxy/orders", s.proxyOrdersList)
	mux.HandleFunc("POST /proxy/orders", s.proxyOrdersCreate)
	mux.HandleFunc("GET /proxy/orders/{id}", s.proxyOrderGet)
	mux.HandleFunc("DELETE /proxy/orders/{id}", s.proxyOrderDelete)
	mux.HandleFunc("PATCH /proxy/orders/{id}/status", s.proxyOrderStatus)

	mux.HandleFunc("POST /demo/crash", s.demoCrash)

	// El orden importa: withRequestID debe fijar el ID en el contexto antes
	// de que withLogging lo lea, así que va inmediatamente por dentro de éste.
	return withRecovery(s.logger, withSecurityHeaders(withRequestID(withLogging(s.logger, mux))))
}
