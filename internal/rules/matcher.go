package rules

import (
	"regexp"
	"strings"
)

type DomainMatcher struct {
	exact  map[string]struct{}
	suffix map[string]struct{}
	regex  []*regexp.Regexp
}

func NewDomainMatcher() *DomainMatcher {
	return &DomainMatcher{
		exact:  make(map[string]struct{}, 256),
		suffix: make(map[string]struct{}, 256),
	}
}

func (m *DomainMatcher) AddExact(d string)             { m.exact[strings.ToLower(d)] = struct{}{} }
func (m *DomainMatcher) AddSuffix(d string)            { m.suffix[strings.ToLower(d)] = struct{}{} }
func (m *DomainMatcher) AddRegex(re *regexp.Regexp)    { m.regex = append(m.regex, re) }

func (m *DomainMatcher) Contains(host string) bool {
	if host == "" {
		return false
	}
	host = strings.ToLower(host)
	if _, ok := m.exact[host]; ok {
		return true
	}
	parts := strings.Split(host, ".")
	for i := 0; i < len(parts); i++ {
		s := strings.Join(parts[i:], ".")
		if _, ok := m.suffix[s]; ok {
			return true
		}
	}
	for _, re := range m.regex {
		if re.MatchString(host) {
			return true
		}
	}
	return false
}