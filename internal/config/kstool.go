package config

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// KSTool holds tenancy / cluster options. Persisted at ~/.kstool/config.yaml.
type KSTool struct {
	Namespace       string   `yaml:"namespace"`
	UserLabel       string   `yaml:"user_label"`
	GPUProducts     []string `yaml:"gpu_products"`
	PriorityClass   []string `yaml:"priority_classes"`
	BaseTemplate    string   `yaml:"base_template_path"` // optional override
	BaseTemplateURL string   `yaml:"base_template_url"`  // fallback fetch URL
	LogsTailLines   int64    `yaml:"logs_tail_lines"`    // 0 means unlimited
	AutoRefreshSec  int      `yaml:"auto_refresh_seconds"` // 0 disables
}

// Default returns the EIDF-shaped defaults the legacy app shipped with.
func Default() KSTool {
	return KSTool{
		Namespace: "eidf029ns",
		UserLabel: "eidf/user",
		GPUProducts: []string{
			"NVIDIA-H200",
			"NVIDIA-H100-80GB-HBM3",
			"NVIDIA-A100-SXM4-80GB",
			"NVIDIA-A100-SXM4-40GB-MIG-3g.20gb",
		},
		PriorityClass: []string{
			"default-workload-priority",
			"batch-workload-priority",
			"short-workload-high-priority",
		},
		BaseTemplateURL: "https://raw.githubusercontent.com/Suchun-sv/KSTool/main/config/base_apply.yaml",
		LogsTailLines:   2000,
		AutoRefreshSec:  10,
	}
}

// Load reads the config file, falling back to defaults if it does not exist.
// A missing file is written with defaults so future edits are easy.
func Load(p Paths) (KSTool, error) {
	data, err := os.ReadFile(p.ConfigFile)
	if errors.Is(err, os.ErrNotExist) {
		cfg := Default()
		if err := Save(p, cfg); err != nil {
			return cfg, err
		}
		return cfg, nil
	}
	if err != nil {
		return KSTool{}, fmt.Errorf("read config: %w", err)
	}
	var cfg KSTool
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return KSTool{}, fmt.Errorf("parse config: %w", err)
	}
	cfg.fillDefaults()
	return cfg, nil
}

// Save persists the config atomically.
func Save(p Paths, cfg KSTool) error {
	cfg.fillDefaults()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return atomicWrite(p.ConfigFile, data)
}

func (c *KSTool) fillDefaults() {
	d := Default()
	if c.Namespace == "" {
		c.Namespace = d.Namespace
	}
	if c.UserLabel == "" {
		c.UserLabel = d.UserLabel
	}
	if len(c.GPUProducts) == 0 {
		c.GPUProducts = d.GPUProducts
	}
	if len(c.PriorityClass) == 0 {
		c.PriorityClass = d.PriorityClass
	}
	if c.BaseTemplateURL == "" {
		c.BaseTemplateURL = d.BaseTemplateURL
	}
	if c.LogsTailLines == 0 {
		c.LogsTailLines = d.LogsTailLines
	}
	if c.AutoRefreshSec == 0 {
		c.AutoRefreshSec = d.AutoRefreshSec
	}
}

// EnsureTemplate writes a base template to disk if missing. When the user has
// pointed BaseTemplate at a local file we copy from there, otherwise we fetch
// BaseTemplateURL once. The download has a hard timeout so a slow network
// can't block startup forever.
func EnsureTemplate(p Paths, cfg KSTool) error {
	if _, err := os.Stat(p.Template); err == nil {
		return nil
	}
	if cfg.BaseTemplate != "" {
		data, err := os.ReadFile(cfg.BaseTemplate)
		if err != nil {
			return fmt.Errorf("read base template: %w", err)
		}
		return atomicWrite(p.Template, data)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(cfg.BaseTemplateURL)
	if err != nil {
		return fmt.Errorf("fetch base template: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch base template: status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read base template body: %w", err)
	}
	return atomicWrite(p.Template, body)
}

// ReadTemplate returns the raw template bytes.
func ReadTemplate(p Paths) ([]byte, error) {
	return os.ReadFile(p.Template)
}
