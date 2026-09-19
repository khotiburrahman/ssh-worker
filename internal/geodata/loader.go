package geodata

import (
	"fmt"
	"os"
	"sync"

	"github.com/youruser/gosshtunnel/internal/logger"
)

type DomainType int

const (
	TypeDomain DomainType = iota
	TypeFull
	TypeRegex
	TypeKeyword
)

func (t DomainType) String() string {
	switch t {
	case TypeDomain:
		return "domain"
	case TypeFull:
		return "full"
	case TypeRegex:
		return "regex"
	case TypeKeyword:
		return "keyword"
	}
	return "unknown"
}

type Domain struct {
	Type  DomainType
	Value string
	Attrs []string
}

type Loader struct {
	mu    sync.RWMutex
	path  string
	sites map[string][]Domain
	log   *logger.Logger
}

type Options struct {
	Path string
	Log  *logger.Logger
}

func New(opts Options) (*Loader, error) {
	lg := opts.Log.For(logger.CompGeodata)

	if _, err := os.Stat(opts.Path); err != nil {
		lg.Errorf(logger.CodeGeoFileMissing, "geosite.dat tidak ditemukan", err,
			"path", opts.Path)
		return nil, err
	}

	l := &Loader{
		path:  opts.Path,
		sites: make(map[string][]Domain),
		log:   lg,
	}

	data, err := os.ReadFile(opts.Path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	sites, err := decodeGeoSiteList(data)
	if err != nil {
		lg.Errorf(logger.CodeGeoDecode, "geosite decode failed", err)
		return nil, fmt.Errorf("decode: %w", err)
	}
	l.sites = sites

	lg.Info("geosite.dat loaded",
		"path", opts.Path,
		"categories", len(sites),
	)
	return l, nil
}

func (l *Loader) Lookup(category string) []Domain {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.sites[category]
}

func (l *Loader) Categories() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]string, 0, len(l.sites))
	for k := range l.sites {
		out = append(out, k)
	}
	return out
}

func (l *Loader) Stats() map[string]int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make(map[string]int, len(l.sites))
	for k, v := range l.sites {
		out[k] = len(v)
	}
	return out
}