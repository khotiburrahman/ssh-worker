package clashapi

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

// ---------- Basic ----------

func (s *Server) handleHello(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{"hello": "clash"})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, VersionResponse{Version: s.opt.Version, Meta: true})
}

// ---------- Configs ----------

func (s *Server) handleGetConfigs(w http.ResponseWriter, r *http.Request) {
	if s.opt.ClashConfig != nil {
		c := s.opt.ClashConfig
		writeJSON(w, 200, ConfigsResponse{
			Port:        c.Port,
			SocksPort:   c.SocksPort,
			RedirPort:   c.RedirPort,
			MixedPort:   c.MixedPort,
			AllowLAN:    c.AllowLAN,
			BindAddress: c.BindAddress,
			Mode:        c.Mode,
			LogLevel:    c.LogLevel,
			IPv6:        c.IPv6,
		})
		return
	}
	socksPort := 0
	if len(s.opt.Workers) > 0 {
		if _, p, err := splitHostPort(s.opt.Workers[0].Addr); err == nil {
			socksPort = p
		}
	}
	writeJSON(w, 200, ConfigsResponse{
		SocksPort:   socksPort,
		MixedPort:   socksPort,
		BindAddress: "127.0.0.1",
		Mode:        "rule",
		LogLevel:    "info",
	})
}

func (s *Server) handlePatchConfigs(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Proxies ----------

func (s *Server) handleGetProxies(w http.ResponseWriter, r *http.Request) {
	proxies := map[string]Proxy{
		"DIRECT": {Type: "Direct", Name: "DIRECT", UDP: true},
		"REJECT": {Type: "Reject", Name: "REJECT", UDP: false},
	}

	var allNames []string
	for _, w := range s.opt.Workers {
		alive := true
		if w.Healthy != nil {
			alive = w.Healthy()
		}
		hist := []DelayHistory{}
		if w.Latency != nil {
			d := w.Latency()
			hist = []DelayHistory{{Time: time.Now(), Delay: int(d.Milliseconds())}}
		}
		extra := map[string]string{"addr": w.Addr}
		if w.Snapshot != nil {
			if b, err := json.Marshal(w.Snapshot()); err == nil {
				extra["health"] = string(b)
			}
		}
		proxies[w.Name] = Proxy{
			Type: "Socks5", Name: w.Name, UDP: false,
			History: hist, Alive: alive, Extra: extra,
		}
		allNames = append(allNames, w.Name)
	}

	now := ""
	if s.opt.ActiveWorker != nil {
		now = s.opt.ActiveWorker()
	}
	if now == "" && len(allNames) > 0 {
		now = allNames[0]
	}
	proxies[s.opt.SelectorName] = Proxy{
		Type: "Selector", Name: s.opt.SelectorName,
		Now: now, All: allNames,
	}

	// Merge YAML proxies & groups
	s.mergeYAMLProxies(proxies)

	writeJSON(w, 200, ProxiesResponse{Proxies: proxies})
}

func (s *Server) handleGetProxy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == s.opt.SelectorName {
		s.handleGetProxies(w, r)
		return
	}
	w2 := s.findWorker(name)
	if w2 == nil {
		writeJSON(w, 404, map[string]string{"error": "Proxy not found"})
		return
	}
	alive := true
	if w2.Healthy != nil {
		alive = w2.Healthy()
	}
	writeJSON(w, 200, Proxy{
		Type: "Socks5", Name: name, Alive: alive,
		Extra: map[string]string{"addr": w2.Addr},
	})
}

func (s *Server) handlePutProxy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name != s.opt.SelectorName {
		writeJSON(w, 400, map[string]string{"error": "not a selector"})
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if s.opt.SetActiveWorker == nil {
		writeJSON(w, 501, map[string]string{"error": "switching not supported"})
		return
	}
	if err := s.opt.SetActiveWorker(body.Name); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.log.Info("active worker switched via API", "worker", body.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProxyDelay(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	w2 := s.findWorker(name)
	if w2 == nil {
		writeJSON(w, 404, map[string]string{"error": "Proxy not found"})
		return
	}
	if w2.Healthy != nil && !w2.Healthy() {
		writeJSON(w, 200, DelayResponse{Delay: 0, MeanDelay: 0})
		return
	}
	d := time.Duration(0)
	if w2.Latency != nil {
		d = w2.Latency()
	}
	writeJSON(w, 200, DelayResponse{
		Delay:     int(d.Milliseconds()),
		MeanDelay: int(d.Milliseconds()),
	})
}

// ---------- Connections ----------

func (s *Server) handleConnections(w http.ResponseWriter, r *http.Request) {
	if isWebSocket(r) {
		s.serveConnectionsWS(w, r)
		return
	}
	writeJSON(w, 200, s.opt.Connections.Snapshot())
}

func (s *Server) handleCloseAllConnections(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCloseConnection(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Traffic ----------

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	if isWebSocket(r) {
		s.serveTrafficWS(w, r)
		return
	}
	up, dn := s.opt.Traffic.Totals()
	writeJSON(w, 200, TrafficResponse{Up: up, Down: dn})
}

// ---------- Logs ----------

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if isWebSocket(r) {
		s.serveLogsWS(w, r)
		return
	}
	writeJSON(w, 200, []LogMessage{})
}

// ---------- WebSocket ----------

func isWebSocket(r *http.Request) bool {
	return r.Header.Get("Upgrade") == "websocket"
}

func (s *Server) serveConnectionsWS(w http.ResponseWriter, r *http.Request) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	ch, unsub := s.opt.Connections.Subscribe()
	defer unsub()

	_ = ws.WriteJSON(s.opt.Connections.Snapshot())

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ch:
			if err := ws.WriteJSON(s.opt.Connections.Snapshot()); err != nil {
				return
			}
		case <-ticker.C:
			if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Server) serveTrafficWS(w http.ResponseWriter, r *http.Request) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			up, dn := s.opt.Traffic.Snapshot()
			if err := ws.WriteJSON(TrafficResponse{Up: up, Down: dn}); err != nil {
				return
			}
		}
	}
}

func (s *Server) serveLogsWS(w http.ResponseWriter, r *http.Request) {
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	ch, unsub := s.opt.Logs.Subscribe()
	defer unsub()

	_ = ws.WriteJSON(LogMessage{Type: "info", Payload: "connected to logs"})

	for {
		select {
		case <-r.Context().Done():
			return
		case lm := <-ch:
			if err := ws.WriteJSON(lm); err != nil {
				return
			}
		}
	}
}

func splitHostPort(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	p, _ := strconv.Atoi(portStr)
	return host, p, nil
}