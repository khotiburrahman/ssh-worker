package rules

import (
	"strings"

	"github.com/youruser/gosshtunnel/internal/logger"
)

type Engine struct {
	rules []Rule
	log   *logger.Logger
}

func NewEngine(log *logger.Logger) *Engine {
	return &Engine{log: log.For(logger.CompRules)}
}

func (e *Engine) Match(ctx MatchContext) MatchResult {
	for _, r := range e.rules {
		if r.Match(ctx) {
			return MatchResult{Adapter: r.Adapter(), MatchedBy: r.Raw()}
		}
	}
	return MatchResult{Adapter: AdapterDirect, MatchedBy: "MATCH,DIRECT (implicit)"}
}

func (e *Engine) Add(r Rule)     { e.rules = append(e.rules, r) }
func (e *Engine) Count() int     { return len(e.rules) }
func (e *Engine) Rules() []Rule  { return e.rules }

func (e *Engine) IsBlocked(ctx MatchContext) bool {
	return e.Match(ctx).Adapter == AdapterReject
}

func NormalizeHost(h string) string {
	h = strings.TrimSuffix(h, ".")
	return strings.ToLower(h)
}