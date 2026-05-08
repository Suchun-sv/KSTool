// Package tui wires the tview-based terminal UI on top of the k8s client.
package tui

import (
	"context"
	"fmt"

	"github.com/rivo/tview"

	"github.com/suchun/kstool/internal/config"
	"github.com/suchun/kstool/internal/k8s"
	klog "github.com/suchun/kstool/internal/log"
)

const (
	appName = "KSTool"
	version = "2.0.1"
	author  = "Beining Yang@LFCS"
)

// App owns the long-lived dependencies the TUI needs.
type App struct {
	app    *tview.Application
	pages  *tview.Pages
	client k8s.Client
	cfg    config.KSTool
	paths  config.Paths
	user   string
	jobs   *jobsView
	ctx    context.Context
	cancel context.CancelFunc
}

// Run launches the TUI and blocks until the user quits.
func Run(ctx context.Context, client k8s.Client, cfg config.KSTool, paths config.Paths) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	tv := tview.NewApplication()
	tv.EnableMouse(true)

	a := &App{
		app:    tv,
		pages:  tview.NewPages(),
		client: client,
		cfg:    cfg,
		paths:  paths,
		user:   klog.CurrentUser(),
		ctx:    cctx,
		cancel: cancel,
	}
	a.jobs = newJobsView(a)
	a.pages.AddPage("jobs", a.jobs.root, true, true)

	a.jobs.scheduleRefresh()
	if err := tv.SetRoot(a.pages, true).SetFocus(a.jobs.table).Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}

// pushModal places a modal on top of the page stack with a unique name and
// returns a function that pops it.
func (a *App) pushModal(p tview.Primitive) func() {
	name := fmt.Sprintf("modal-%p", p)
	return a.pushPage(name, p)
}

// pushPage adds a named page on top of the stack and returns its pop func.
// If a page with the given name already exists it is removed first so the
// stack stays consistent across re-renders.
func (a *App) pushPage(name string, p tview.Primitive) func() {
	if a.pages.HasPage(name) {
		a.pages.RemovePage(name)
	}
	a.pages.AddPage(name, p, true, true)
	a.app.SetFocus(p)
	return func() {
		a.pages.RemovePage(name)
		if _, prim := a.pages.GetFrontPage(); prim != nil {
			a.app.SetFocus(prim)
		}
	}
}

// showError displays an error modal that dismisses with OK.
func (a *App) showError(err error) {
	if err == nil {
		return
	}
	a.showModal("⚠️  "+err.Error(), []string{"OK"}, nil)
}

// showMessage shows an info modal.
func (a *App) showMessage(msg string) {
	a.showModal(msg, []string{"OK"}, nil)
}

// showModal builds a tview.Modal with the given buttons and a per-button
// callback (nil means just dismiss).
func (a *App) showModal(text string, buttons []string, onChoice func(label string)) {
	modal := tview.NewModal().SetText(text).AddButtons(buttons)
	var pop func()
	modal.SetDoneFunc(func(_ int, label string) {
		if pop != nil {
			pop()
		}
		if onChoice != nil {
			onChoice(label)
		}
	})
	pop = a.pushModal(modal)
}

func versionLine() string {
	return fmt.Sprintf("%s@%s by %s", appName, version, author)
}
