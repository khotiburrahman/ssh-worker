package clashapi

import (
	"net/http"

	"github.com/youruser/gosshtunnel/internal/rules"
)

type ruleEntry struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
	Proxy   string `json:"proxy"`
}

type rulesResponse struct {
	Rules []ruleEntry `json:"rules"`
}

func (s *Server) handleGetRules(w http.ResponseWriter, r *http.Request) {
	if s.opt.RuleEngine == nil {
		writeJSON(w, 200, rulesResponse{Rules: []ruleEntry{}})
		return
	}
	raw := s.opt.RuleEngine.Rules()
	out := make([]ruleEntry, 0, len(raw))
	for _, r := range raw {
		out = append(out, ruleEntry{
			Type:    r.Type(),
			Payload: r.Payload(),
			Proxy:   string(r.Adapter()),
		})
	}
	writeJSON(w, 200, rulesResponse{Rules: out})
}

func (s *Server) handleGetBlocklist(w http.ResponseWriter, r *http.Request) {
	if s.opt.RuleEngine == nil {
		writeJSON(w, 200, rulesResponse{Rules: []ruleEntry{}})
		return
	}
	out := []ruleEntry{}
	for _, r := range s.opt.RuleEngine.Rules() {
		if r.Adapter() == rules.AdapterReject {
			out = append(out, ruleEntry{
				Type: r.Type(), Payload: r.Payload(), Proxy: "REJECT",
			})
		}
	}
	writeJSON(w, 200, rulesResponse{Rules: out})
}