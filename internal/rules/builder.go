package rules

import (
	"github.com/youruser/gosshtunnel/internal/clashconfig"
	"github.com/youruser/gosshtunnel/internal/geodata"
	"github.com/youruser/gosshtunnel/internal/logger"
)

type BuildOptions struct {
	GeoSitePath string
	Config      *clashconfig.ClashConfig
}

func Build(opts BuildOptions, log *logger.Logger) (*Engine, *GeoSiteStore, error) {
	var store *GeoSiteStore
	if opts.GeoSitePath != "" {
		loader, err := geodata.New(geodata.Options{
			Path: opts.GeoSitePath,
			Log:  log,
		})
		if err != nil {
			return nil, nil, err
		}
		store = NewGeoSiteStore(loader, log)
	} else {
		store = NewGeoSiteStore(nil, log)
	}

	eng := NewEngine(log)
	if opts.Config == nil {
		return eng, store, nil
	}

	lg := log.For(logger.CompRules)
	for _, line := range opts.Config.Rules {
		r, err := Parse(line, store)
		if err != nil {
			lg.Warn("rule di-skip",
				"code", logger.CodeRulParse,
				"line", line, "err", err)
			continue
		}
		if r == nil {
			continue
		}
		eng.Add(r)
	}
	lg.Info("rule engine ready", "total", eng.Count())
	return eng, store, nil
}