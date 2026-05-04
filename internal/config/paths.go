// Package config owns ~/.kstool: the global tenancy config and the
// per-template env-var configs saved under env_config_list/. All writes are
// atomic (tempfile + rename) and use 0600 permissions.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

const (
	dirName        = ".kstool"
	configFileName = "config.yaml"
	templateName   = "base_apply.yaml"
	envListDir     = "env_config_list"
)

var safeName = regexp.MustCompile(`^[a-zA-Z0-9_.\-]+$`)

// Paths resolves filesystem locations under ~/.kstool. Construct via Resolve.
type Paths struct {
	Home        string
	Root        string
	ConfigFile  string
	Template    string
	EnvListDir  string
}

// Resolve constructs Paths and ensures every directory exists with 0700.
func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("locate home dir: %w", err)
	}
	root := filepath.Join(home, dirName)
	p := Paths{
		Home:       home,
		Root:       root,
		ConfigFile: filepath.Join(root, configFileName),
		Template:   filepath.Join(root, templateName),
		EnvListDir: filepath.Join(root, envListDir),
	}
	for _, d := range []string{p.Root, p.EnvListDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return Paths{}, fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return p, nil
}

// EnvConfigPath returns the on-disk location for a saved env config. The name
// is validated against a strict allowlist to prevent path traversal.
func (p Paths) EnvConfigPath(name string) (string, error) {
	if !safeName.MatchString(name) {
		return "", fmt.Errorf("invalid config name %q (allowed: alphanumerics, _, -, .)", name)
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("invalid config name %q", name)
	}
	return filepath.Join(p.EnvListDir, name+".yaml"), nil
}

// atomicWrite writes data to path via a temp file in the same directory and a
// rename, with 0600 perms. Returns the same data unchanged on read errors.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".kstool-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}
