package rules

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

func Parse(line string, geoSites *GeoSiteStore) (Rule, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil, nil
	}
	parts := splitCSV(line)
	if len(parts) < 2 {
		return nil, fmt.Errorf("rule invalid: %q", line)
	}
	typ := strings.ToUpper(parts[0])

	switch typ {
	case "DOMAIN":
		if len(parts) < 3 {
			return nil, errMalformed(line)
		}
		return &DomainRule{domain: strings.ToLower(parts[1]), adapter: Adapter(parts[2]), raw: line}, nil

	case "DOMAIN-SUFFIX":
		if len(parts) < 3 {
			return nil, errMalformed(line)
		}
		return &DomainSuffixRule{suffix: strings.ToLower(parts[1]), adapter: Adapter(parts[2]), raw: line}, nil

	case "DOMAIN-KEYWORD":
		if len(parts) < 3 {
			return nil, errMalformed(line)
		}
		return &DomainKeywordRule{keyword: parts[1], adapter: Adapter(parts[2]), raw: line}, nil

	case "DOMAIN-REGEX":
		if len(parts) < 3 {
			return nil, errMalformed(line)
		}
		re, err := regexp.Compile(parts[1])
		if err != nil {
			return nil, fmt.Errorf("regex invalid: %w", err)
		}
		return &DomainRegexRule{re: re, pattern: parts[1], adapter: Adapter(parts[2]), raw: line}, nil

	case "IP-CIDR", "IP-CIDR6":
		if len(parts) < 3 {
			return nil, errMalformed(line)
		}
		_, ipnet, err := net.ParseCIDR(parts[1])
		if err != nil {
			return nil, fmt.Errorf("cidr invalid: %w", err)
		}
		noResolve := len(parts) >= 4 && strings.EqualFold(parts[3], "no-resolve")
		return &IPCIDRRule{network: ipnet, noResolve: noResolve, adapter: Adapter(parts[2]), raw: line}, nil

	case "DST-PORT":
		if len(parts) < 3 {
			return nil, errMalformed(line)
		}
		port := atoi(parts[1])
		if port <= 0 {
			return nil, fmt.Errorf("port invalid: %s", parts[1])
		}
		return &DstPortRule{port: port, adapter: Adapter(parts[2]), raw: line}, nil

	case "GEOSITE":
		if len(parts) < 3 {
			return nil, errMalformed(line)
		}
		category := parts[1]
		if geoSites == nil {
			return nil, fmt.Errorf("geosite store tidak tersedia")
		}
		matcher := geoSites.Get(category)
		if matcher == nil {
			return nil, fmt.Errorf("geosite %q tidak dikenal", category)
		}
		return &GeoSiteRule{
			category: category, adapter: Adapter(parts[2]),
			raw: line, matcher: matcher,
		}, nil

	case "MATCH":
		if len(parts) < 2 {
			return nil, errMalformed(line)
		}
		return &MatchRule{adapter: Adapter(parts[1]), raw: line}, nil

	case "GEOIP", "RULE-SET", "PROCESS-NAME", "SRC-IP-CIDR", "SRC-PORT":
		return nil, nil

	default:
		return nil, nil
	}
}

func errMalformed(line string) error {
	return fmt.Errorf("rule malformed: %q", line)
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}