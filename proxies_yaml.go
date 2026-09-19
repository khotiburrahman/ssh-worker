package clashapi

import "strconv"

func (s *Server) mergeYAMLProxies(proxies map[string]Proxy) {
	if s.opt.ClashConfig == nil {
		return
	}
	for _, p := range s.opt.ClashConfig.Proxies {
		proxies[p.Name] = Proxy{
			Type: "Socks5",
			Name: p.Name,
			UDP:  p.UDP,
			Extra: map[string]string{
				"server": p.Server,
				"port":   strconv.Itoa(p.Port),
				"source": "yaml",
			},
		}
	}
	for _, g := range s.opt.ClashConfig.ProxyGroups {
		p := Proxy{
			Type: mapGroupType(g.Type),
			Name: g.Name,
			All:  g.Proxies,
		}
		if g.Type == "select" && len(g.Proxies) > 0 {
			p.Now = g.Proxies[0]
		}
		proxies[g.Name] = p
	}
}

func mapGroupType(t string) string {
	switch t {
	case "select":
		return "Selector"
	case "url-test":
		return "URLTest"
	case "fallback":
		return "Fallback"
	case "load-balance":
		return "LoadBalance"
	}
	return "Selector"
}