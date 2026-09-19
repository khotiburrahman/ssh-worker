package clashconfig

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/youruser/gosshtunnel/internal/logger"
)

func Load(path string, log *logger.Logger) (*ClashConfig, error) {
	lg := log.For(logger.CompClashCfg)
	lg.Info("loading clash yaml", "path", path)

	data, err := os.ReadFile(path)
	if err != nil {
		lg.Errorf(logger.CodeCfgLoadYAML, "yaml read failed", err)
		return nil, err
	}
	var c ClashConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		lg.Errorf(logger.CodeCfgLoadYAML, "yaml parse failed", err)
		return nil, err
	}
	c.applyDefaults(path)
	lg.Info("clash yaml loaded",
		"mixed_port", c.MixedPort,
		"proxies", len(c.Proxies),
		"groups", len(c.ProxyGroups),
		"rules", len(c.Rules),
	)
	return &c, nil
}

func (c *ClashConfig) applyDefaults(yamlPath string) {
	if c.Mode == "" {
		c.Mode = "rule"
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	if c.ExternalController == "" {
		c.ExternalController = "127.0.0.1:9090"
	}
	if c.ExternalUI != "" && !filepath.IsAbs(c.ExternalUI) {
		c.ExternalUI = filepath.Join(filepath.Dir(yamlPath), c.ExternalUI)
	}
}

func (c *ClashConfig) Validate() error {
	if c.MixedPort < 0 || c.SocksPort < 0 || c.Port < 0 {
		return errors.New("clashconfig: port tidak boleh negatif")
	}
	known := map[string]bool{"DIRECT": true, "REJECT": true, "GLOBAL": true}
	for _, p := range c.Proxies {
		known[p.Name] = true
	}
	for _, g := range c.ProxyGroups {
		known[g.Name] = true
	}
	for _, g := range c.ProxyGroups {
		for _, ref := range g.Proxies {
			if !known[ref] {
				return errors.New("clashconfig: group " + g.Name + " merujuk proxy tak dikenal: " + ref)
			}
		}
	}
	return nil
}