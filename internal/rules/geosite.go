package rules

import (
	"regexp"

	"github.com/youruser/gosshtunnel/internal/geodata"
	"github.com/youruser/gosshtunnel/internal/logger"
)

type GeoSiteStore struct {
	loader *geodata.Loader
	log    *logger.Logger
	cache  map[string]*DomainMatcher
}

func NewGeoSiteStore(loader *geodata.Loader, log *logger.Logger) *GeoSiteStore {
	return &GeoSiteStore{
		loader: loader,
		log:    log.For(logger.CompRules),
		cache:  make(map[string]*DomainMatcher),
	}
}

func (s *GeoSiteStore) Get(category string) *DomainMatcher {
	if m, ok := s.cache[category]; ok {
		return m
	}
	if s.loader == nil {
		return nil
	}
	domains := s.loader.Lookup(category)
	if domains == nil {
		return nil
	}

	m := NewDomainMatcher()
	for _, d := range domains {
		switch d.Type {
		case geodata.TypeDomain:
			m.AddSuffix(d.Value)
		case geodata.TypeFull:
			m.AddExact(d.Value)
		case geodata.TypeKeyword:
			m.AddSuffix(d.Value)
		case geodata.TypeRegex:
			re, err := regexp.Compile(d.Value)
			if err != nil {
				s.log.Warn("geosite regex invalid",
					"category", category, "pattern", d.Value, "err", err)
				continue
			}
			m.AddRegex(re)
		}
	}
	s.cache[category] = m
	s.log.Debug("geosite category cached",
		"category", category, "entries", len(domains))
	return m
}