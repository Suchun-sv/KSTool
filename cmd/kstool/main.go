// Command kstool is a tview-based TUI for managing Kubernetes Jobs.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/suchun/kstool/internal/config"
	"github.com/suchun/kstool/internal/k8s"
	"github.com/suchun/kstool/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "kstool:", err)
		os.Exit(1)
	}
}

func run() error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	cfg, err := config.Load(paths)
	if err != nil {
		return err
	}
	if err := config.EnsureTemplate(paths, cfg); err != nil {
		return fmt.Errorf("ensure template: %w", err)
	}
	client, err := k8s.New(cfg.Namespace, cfg.UserLabel)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	return tui.Run(ctx, client, cfg, paths)
}
