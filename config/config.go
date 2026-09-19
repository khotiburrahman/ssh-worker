package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/youruser/gosshtunnel/internal/logger"
)

type Config struct {
	Listen      ListenConfig      `json:"listen"`
	Concurrency ConcurrencyConfig `json:"concurrency"`
	SSH         SSHConfig         `json:"ssh"`
	Network     NetworkConfig     `json:"network"`
	Transport   TransportConfig   `json:"transport"`
	Proxy       ProxyConfig       `json:"proxy"`
	Payload     PayloadConfig     `json:"payload"`

	Health   HealthConfig  `json:"health,omitempty"`
	SOCKS5   SOCKS5Options `json:"socks5,omitempty"`
	ClashAPI ClashAPIConfig `json:"clash_api,omitempty"`
	Rules    RuleSourceConfig `json:"rules,omitempty"`

	LogLevel   string `json:"log_level,omitempty"`
	KnownHosts string `json:"known_hosts,omitempty"`
}

type ListenConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type ConcurrencyConfig struct {
	Enable    bool `json:"enable"`
	Workers   int  `json:"workers"`
	StartPort int  `json:"start_port"`
}

type SSHConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	PrivateKey string `json:"private_key,omitempty"`
}

type NetworkConfig struct {
	Type string `json:"type"`
}

type TransportConfig struct {
	TLS  bool   `json:"tls"`
	Host string `json:"host"`
	Path string `json:"path"`
	SNI  string `json:"sni"`
}

type ProxyConfig struct {
	Enable   bool   `json:"enable"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Type     string `json:"type,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type PayloadConfig struct {
	Enable  bool     `json:"enable"`
	Request string   `json:"request"`
	Expect  []string `json:"expect"`
}

type HealthConfig struct {
	DegradedRTT  time.Duration `json:"degraded_rtt,omitempty"`
	UnhealthyRTT time.Duration `json:"unhealthy_rtt,omitempty"`
	DegradedErr  float64       `json:"degraded_err,omitempty"`
	UnhealthyErr float64       `json:"unhealthy_err,omitempty"`
	DegradedHold time.Duration `json:"degraded_hold,omitempty"`
	HealHold     time.Duration `json:"heal_hold,omitempty"`
}

type SOCKS5Options struct {
	MaxConns    int           `json:"max_conns,omitempty"`
	IdleTimeout time.Duration `json:"idle_timeout,omitempty"`
}

type ClashAPIConfig struct {
	Enable      bool   `json:"enable"`
	Listen      string `json:"listen"`
	Secret      string `json:"secret,omitempty"`
	CORSOrigins string `json:"cors_origins,omitempty"`
}

type RuleSourceConfig struct {
	GeoSitePath string `json:"geosite_path,omitempty"`
}

// ---------- Loader ----------

func Load(path string, log *logger.Logger) (*Config, error) {
	lg := log.For(logger.CompConfig)

	data, err := os.ReadFile(path)
	if err != nil {
		lg.Errorf(logger.CodeCfgLoad, "config read failed", err, "path", path)
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		lg.Errorf(logger.CodeCfgLoad, "config parse failed", err, "path", path)
		return nil, fmt.Errorf("parse json: %w", err)
	}
	lg.Info("config loaded",
		"path", path,
		"ssh_host", c.SSH.Host,
		"proxy_enable", c.Proxy.Enable,
		"payload_enable", c.Payload.Enable,
		"workers", c.Concurrency.Workers,
	)
	return &c, nil
}

// ---------- Helpers ----------

func (c *Config) DialTarget() string {
	if c.Proxy.Enable && c.Proxy.Host != "" {
		return net.JoinHostPort(c.Proxy.Host, strconv.Itoa(c.Proxy.Port))
	}
	return net.JoinHostPort(c.SSH.Host, strconv.Itoa(c.SSH.Port))
}

func (c *Config) PayloadHost() string {
	if c.Transport.Host != "" {
		return c.Transport.Host
	}
	return c.SSH.Host
}

func (c *Config) SNIHost() string {
	if c.Transport.SNI != "" {
		return c.Transport.SNI
	}
	if c.Transport.Host != "" {
		return c.Transport.Host
	}
	return c.SSH.Host
}

func (c *Config) ListenAddrs() []string {
	host := c.Listen.Host
	if host == "" {
		host = "127.0.0.1"
	}
	if !c.Concurrency.Enable || c.Concurrency.Workers <= 1 {
		return []string{net.JoinHostPort(host, strconv.Itoa(c.Listen.Port))}
	}
	start := c.Concurrency.StartPort
	if start == 0 {
		start = c.Listen.Port
	}
	addrs := make([]string, 0, c.Concurrency.Workers)
	for i := 0; i < c.Concurrency.Workers; i++ {
		addrs = append(addrs, net.JoinHostPort(host, strconv.Itoa(start+i)))
	}
	return addrs
}

// ---------- Defaults & Validate ----------

func (c *Config) ApplyDefaults() {
	if c.Network.Type == "" {
		c.Network.Type = "tcp"
	}
	if c.Transport.Path == "" {
		c.Transport.Path = "/"
	}
	if c.Proxy.Type == "" {
		c.Proxy.Type = "http"
	}
	if c.Concurrency.Workers <= 0 {
		c.Concurrency.Workers = 1
	}
	if c.SSH.Port == 0 {
		c.SSH.Port = 22
	}
	if c.SOCKS5.MaxConns <= 0 {
		c.SOCKS5.MaxConns = 2048
	}
	if c.SOCKS5.IdleTimeout <= 0 {
		c.SOCKS5.IdleTimeout = 20 * time.Second
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
}

func (c *Config) ValidateAll() []error {
	var errs []error
	if c.SSH.Host == "" {
		errs = append(errs, errors.New("ssh.host wajib"))
	}
	if c.SSH.Username == "" {
		errs = append(errs, errors.New("ssh.username wajib"))
	}
	if c.SSH.Password == "" && c.SSH.PrivateKey == "" {
		errs = append(errs, errors.New("ssh: isi password atau private_key"))
	}
	if c.Proxy.Enable && c.Proxy.Host == "" {
		errs = append(errs, errors.New("proxy.enable=true tapi proxy.host kosong"))
	}
	if c.Payload.Enable && c.Payload.Request == "" {
		errs = append(errs, errors.New("payload.enable=true tapi payload.request kosong"))
	}
	if c.Listen.Port == 0 && !c.Concurrency.Enable {
		errs = append(errs, errors.New("listen.port wajib"))
	}
	switch c.Network.Type {
	case "tcp", "ws":
	default:
		errs = append(errs, fmt.Errorf("network.type tidak didukung: %s", c.Network.Type))
	}
	switch c.Proxy.Type {
	case "", "http", "socks5":
	default:
		errs = append(errs, fmt.Errorf("proxy.type tidak didukung: %s", c.Proxy.Type))
	}
	return errs
}