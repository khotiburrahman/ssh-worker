package rules

import (
	"net"
	"regexp"
	"strings"
)

type Adapter string

const (
	AdapterDirect Adapter = "DIRECT"
	AdapterReject Adapter = "REJECT"
)

type MatchContext struct {
	Host    string
	IP      net.IP
	Port    int
	Network string
	SrcIP   net.IP
}

type MatchResult struct {
	Adapter   Adapter
	MatchedBy string
}

type Rule interface {
	Match(ctx MatchContext) bool
	Type() string
	Payload() string
	Adapter() Adapter
	Raw() string
}

// -------- DOMAIN --------

type DomainRule struct {
	domain  string
	adapter Adapter
	raw     string
}

func (r *DomainRule) Match(ctx MatchContext) bool { return ctx.Host == r.domain }
func (r *DomainRule) Type() string                 { return "DOMAIN" }
func (r *DomainRule) Payload() string              { return r.domain }
func (r *DomainRule) Adapter() Adapter             { return r.adapter }
func (r *DomainRule) Raw() string                  { return r.raw }

// -------- DOMAIN-SUFFIX --------

type DomainSuffixRule struct {
	suffix  string
	adapter Adapter
	raw     string
}

func (r *DomainSuffixRule) Match(ctx MatchContext) bool {
	h := ctx.Host
	return h == r.suffix || strings.HasSuffix(h, "."+r.suffix)
}
func (r *DomainSuffixRule) Type() string    { return "DOMAIN-SUFFIX" }
func (r *DomainSuffixRule) Payload() string { return r.suffix }
func (r *DomainSuffixRule) Adapter() Adapter { return r.adapter }
func (r *DomainSuffixRule) Raw() string     { return r.raw }

// -------- DOMAIN-KEYWORD --------

type DomainKeywordRule struct {
	keyword string
	adapter Adapter
	raw     string
}

func (r *DomainKeywordRule) Match(ctx MatchContext) bool {
	return strings.Contains(ctx.Host, r.keyword)
}
func (r *DomainKeywordRule) Type() string    { return "DOMAIN-KEYWORD" }
func (r *DomainKeywordRule) Payload() string { return r.keyword }
func (r *DomainKeywordRule) Adapter() Adapter { return r.adapter }
func (r *DomainKeywordRule) Raw() string     { return r.raw }

// -------- DOMAIN-REGEX --------

type DomainRegexRule struct {
	re      *regexp.Regexp
	pattern string
	adapter Adapter
	raw     string
}

func (r *DomainRegexRule) Match(ctx MatchContext) bool { return r.re.MatchString(ctx.Host) }
func (r *DomainRegexRule) Type() string                { return "DOMAIN-REGEX" }
func (r *DomainRegexRule) Payload() string             { return r.pattern }
func (r *DomainRegexRule) Adapter() Adapter            { return r.adapter }
func (r *DomainRegexRule) Raw() string                 { return r.raw }

// -------- IP-CIDR --------

type IPCIDRRule struct {
	network   *net.IPNet
	noResolve bool
	adapter   Adapter
	raw       string
}

func (r *IPCIDRRule) Match(ctx MatchContext) bool {
	if ctx.IP == nil {
		return false
	}
	return r.network.Contains(ctx.IP)
}
func (r *IPCIDRRule) Type() string {
	if r.network.IP.To4() != nil {
		return "IP-CIDR"
	}
	return "IP-CIDR6"
}
func (r *IPCIDRRule) Payload() string { return r.network.String() }
func (r *IPCIDRRule) Adapter() Adapter { return r.adapter }
func (r *IPCIDRRule) Raw() string     { return r.raw }

// -------- DST-PORT --------

type DstPortRule struct {
	port    int
	adapter Adapter
	raw     string
}

func (r *DstPortRule) Match(ctx MatchContext) bool { return ctx.Port == r.port }
func (r *DstPortRule) Type() string                { return "DST-PORT" }
func (r *DstPortRule) Payload() string             { return itoa(r.port) }
func (r *DstPortRule) Adapter() Adapter            { return r.adapter }
func (r *DstPortRule) Raw() string                 { return r.raw }

// -------- GEOSITE --------

type GeoSiteRule struct {
	category string
	adapter  Adapter
	raw      string
	matcher  *DomainMatcher
}

func (r *GeoSiteRule) Match(ctx MatchContext) bool { return r.matcher.Contains(ctx.Host) }
func (r *GeoSiteRule) Type() string                { return "GEOSITE" }
func (r *GeoSiteRule) Payload() string             { return r.category }
func (r *GeoSiteRule) Adapter() Adapter            { return r.adapter }
func (r *GeoSiteRule) Raw() string                 { return r.raw }

// -------- MATCH --------

type MatchRule struct {
	adapter Adapter
	raw     string
}

func (r *MatchRule) Match(ctx MatchContext) bool { return true }
func (r *MatchRule) Type() string                { return "MATCH" }
func (r *MatchRule) Payload() string             { return "" }
func (r *MatchRule) Adapter() Adapter            { return r.adapter }
func (r *MatchRule) Raw() string                 { return r.raw }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}