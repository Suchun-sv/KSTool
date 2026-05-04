package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnvVar is one entry in a saved env-var configuration.
type EnvVar struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

// EnvConfig is a list of env-var bindings persisted under env_config_list/.
type EnvConfig struct {
	EnvVars []EnvVar `yaml:"env_vars"`
}

// Get returns the value for key, plus whether it was present.
func (c *EnvConfig) Get(key string) (string, bool) {
	for _, e := range c.EnvVars {
		if e.Key == key {
			return e.Value, true
		}
	}
	return "", false
}

// Set inserts or overwrites a key.
func (c *EnvConfig) Set(key, value string) {
	for i := range c.EnvVars {
		if c.EnvVars[i].Key == key {
			c.EnvVars[i].Value = value
			return
		}
	}
	c.EnvVars = append(c.EnvVars, EnvVar{Key: key, Value: value})
}

// AsMap renders the bindings as a map for the template engine.
func (c *EnvConfig) AsMap() map[string]string {
	m := make(map[string]string, len(c.EnvVars))
	for _, e := range c.EnvVars {
		m[e.Key] = e.Value
	}
	return m
}

// Sorted returns a copy with EnvVars ordered by key.
func (c *EnvConfig) Sort() {
	sort.Slice(c.EnvVars, func(i, j int) bool { return c.EnvVars[i].Key < c.EnvVars[j].Key })
}

// ListEnvConfigs returns the names of saved configs (without .yaml extension),
// sorted alphabetically.
func ListEnvConfigs(p Paths) ([]string, error) {
	entries, err := os.ReadDir(p.EnvListDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read env config dir: %w", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ".yaml"))
	}
	sort.Strings(out)
	return out, nil
}

// LoadEnvConfig reads a saved env config by name.
func LoadEnvConfig(p Paths, name string) (*EnvConfig, error) {
	path, err := p.EnvConfigPath(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read env config: %w", err)
	}
	var cfg EnvConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse env config: %w", err)
	}
	return &cfg, nil
}

// SaveEnvConfig writes a saved env config to disk atomically with 0600 perms.
func SaveEnvConfig(p Paths, name string, cfg *EnvConfig) error {
	path, err := p.EnvConfigPath(name)
	if err != nil {
		return err
	}
	cfg.Sort()
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal env config: %w", err)
	}
	return atomicWrite(path, data)
}

// DeleteEnvConfig removes a saved env config.
func DeleteEnvConfig(p Paths, name string) error {
	path, err := p.EnvConfigPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete env config: %w", err)
	}
	return nil
}
