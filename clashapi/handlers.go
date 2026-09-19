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
	if err := s.opt.SetActive