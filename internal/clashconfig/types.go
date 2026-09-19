package clashconfig

type ClashConfig struct {
	MixedPort   int    `yaml:"mixed-port"`
	RedirPort   int    `yaml:"redir-port"`
	SocksPort   int    `yaml:"socks-port"`
	Port        int    `yaml:"port"`
	AllowLAN    bool   `yaml:"allow-lan"`
	BindAddress string `yaml:"bind-address"`
	Mode        string `yaml:"mode"`
	LogLevel    string `yaml:"log-level"`
	IPv6        bool   `yaml:"ipv6"`

	ExternalController string `yaml:"external-controller"`
	ExternalUI         string `yaml:"external-ui"`
	Secret             string `yaml:"secret"`

	GlobalClientFingerprint string `yaml:"global-client-fingerprint"`
	UnifiedDelay            bool   `yaml:"unified-delay"`
	TCPConcurrent           bool   `yaml:"tcp-concurrent"`
	FindProcessMode         string `yaml:"find-process-mode"`

	Sniffer       *Sniffer                `yaml:"sniffer,omitempty"`
	Proxies       []ProxyEntry            `yaml:"proxies"`
	ProxyGroups   []ProxyGroup            `yaml:"proxy-groups"`
	DNS           *DNSConfig              `yaml:"dns,omitempty"`
	Rules         []string                `yaml:"rules"`
	RuleProviders map[string]RuleProvider `yaml:"rule-providers,omitempty"`
}

type ProxyEntry struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Server   string `yaml:"server"`
	Port     int    `yaml:"port"`
	UDP      bool   `yaml:"udp"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type ProxyGroup struct {
	Name      string   `yaml:"name"`
	Type      string   `yaml:"type"`
	Proxies   []string `yaml:"proxies"`
	URL       string   `yaml:"url,omitempty"`
	Interval  int      `yaml:"interval,omitempty"`
	Tolerance int      `yaml:"tolerance,omitempty"`
	Lazy      bool     `yaml:"lazy,omitempty"`
	Strategy  string   `yaml:"strategy,omitempty"`
}

type Sniffer struct {
	Enable          bool                     `yaml:"enable"`
	Sniff           map[string]SniffProtocol `yaml:"sniff"`
	ForceDNSMapping bool                     `yaml:"force-dns-mapping"`
	ParsePureIP     bool                     `yaml:"parse-pure-ip"`
}

type SniffProtocol struct {
	Ports []string `yaml:"ports"`
}

type DNSConfig struct {
	Enable            bool            `yaml:"enable"`
	IPv6              bool            `yaml:"ipv6"`
	Listen            string          `yaml:"listen"`
	EnhancedMode      string          `yaml:"enhanced-mode"`
	FakeIPRange       string          `yaml:"fake-ip-range"`
	FakeIPFilter      []string        `yaml:"fake-ip-filter"`
	DefaultNameserver []string        `yaml:"default-nameserver"`
	Nameserver        []string        `yaml:"nameserver"`
	ProxyServerNS     []string        `yaml:"proxy-server-nameserver"`
	Fallback          []string        `yaml:"fallback"`
	FallbackFilter    *FallbackFilter `yaml:"fallback-filter,omitempty"`
}

type FallbackFilter struct {
	GeoIP  bool     `yaml:"geoip"`
	IPCIDR []string `yaml:"ipcidr"`
}

type RuleProvider struct {
	Type     string `yaml:"type"`
	Behavior string `yaml:"behavior"`
	URL      string `yaml:"url,omitempty"`
	Path     string `yaml:"path,omitempty"`
	Interval int    `yaml:"interval,omitempty"`
}