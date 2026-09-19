package clashapi

import "net/http"

func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /", s.handleHello)
	mux.HandleFunc("GET /version", s.handleVersion)
	mux.HandleFunc("GET /configs", s.handleGetConfigs)
	mux.HandleFunc("PATCH /configs", s.handlePatchConfigs)

	mux.HandleFunc("GET /proxies", s.handleGetProxies)
	mux.HandleFunc("GET /proxies/{name}", s.handleGetProxy)
	mux.HandleFunc("PUT /proxies/{name}", s.handlePutProxy)
	mux.HandleFunc("GET /proxies/{name}/delay", s.handleProxyDelay)

	mux.HandleFunc("GET /connections", s.handleConnections)
	mux.HandleFunc("DELETE /connections", s.handleCloseAllConnections)
	mux.HandleFunc("DELETE /connections/{id}", s.handleCloseConnection)

	mux.HandleFunc("GET /traffic", s.handleTraffic)
	mux.HandleFunc("GET /logs", s.handleLogs)

	mux.HandleFunc("GET /rules", s.handleGetRules)
	mux.HandleFunc("GET /blocklist", s.handleGetBlocklist)
}