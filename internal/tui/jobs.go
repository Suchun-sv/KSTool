package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/suchun/kstool/internal/editor"
	klog "github.com/suchun/kstool/internal/log"
	"github.com/suchun/kstool/internal/model"
)

const (
	refreshThrottle = 2 * time.Second
	listTimeout     = 8 * time.Second
)

type jobsView struct {
	app *App

	root        *tview.Flex
	table       *tview.Table
	statusBar   *tview.TextView
	searchInput *tview.InputField

	jobs        []model.Job
	filter      model.FilterMode
	sortMode    model.SortMode
	onlyOwn     bool
	searchTerm  string
	lastRefresh time.Time
	refreshing  bool
}

func newJobsView(app *App) *jobsView {
	v := &jobsView{app: app, sortMode: model.SortAgeDesc}

	header := tview.NewTextView().
		SetTextAlign(tview.AlignLeft).
		SetText(banner())
	header.SetTextColor(tcell.ColorBlue)

	v.statusBar = tview.NewTextView().SetTextAlign(tview.AlignLeft)
	v.statusBar.SetTextColor(tcell.ColorWhite)

	v.table = tview.NewTable().SetBorders(false).SetSelectable(true, false).SetSeparator(' ')
	v.writeHeaders()
	v.table.SetInputCapture(v.handleKey)

	v.searchInput = tview.NewInputField().SetLabel("/ search: ").SetFieldWidth(0)
	v.searchInput.SetChangedFunc(func(text string) {
		v.searchTerm = text
		v.renderTable()
	})
	v.searchInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			v.searchTerm = ""
			v.searchInput.SetText("")
		}
		v.closeSearch()
	})

	versionTV := tview.NewTextView().SetTextAlign(tview.AlignLeft).SetText(versionLine())

	v.root = tview.NewFlex().SetDirection(tview.FlexRow)
	v.root.AddItem(header, 7, 0, false)
	v.root.AddItem(v.statusBar, 1, 0, false)
	v.root.AddItem(v.searchInput, 0, 0, false)
	v.root.AddItem(v.table, 0, 1, true)
	v.root.AddItem(versionTV, 1, 0, false)

	v.renderStatus()
	go v.autoRefreshLoop()
	return v
}

// autoRefreshLoop ticks every cfg.AutoRefreshSec and triggers a refresh. The
// refresh itself is throttled, so a noisy tick is harmless. Stops when the
// app's context is cancelled.
func (v *jobsView) autoRefreshLoop() {
	if v.app.cfg.AutoRefreshSec <= 0 {
		return
	}
	t := time.NewTicker(time.Duration(v.app.cfg.AutoRefreshSec) * time.Second)
	defer t.Stop()
	for {
		select {
		case <-v.app.ctx.Done():
			return
		case <-t.C:
			v.app.app.QueueUpdateDraw(func() { v.scheduleRefresh() })
		}
	}
}

func banner() string {
	return ` ██╗  ██╗███████╗████████╗ ██████╗  ██████╗ ██╗
 ██║ ██╔╝██╔════╝╚══██╔══╝██╔═══██╗██╔═══██╗██║
 ██████╔╝███████╗   ██║   ██║   ██║██║   ██║██║
 ██╔═██╗ ╚════██║   ██║   ██║   ██║██║   ██║██║
 ██║  ██╗███████║   ██║   ╚██████╔╝╚██████╔╝███████╗
 ╚═╝  ╚═╝╚══════╝   ╚═╝    ╚═════╝  ╚═════╝ ╚══════╝
(d)el (r)efresh (e)xec (l)ogs (i)nfo (c)onfig (n)ew (/)search (?)help (q)uit`
}

func (v *jobsView) writeHeaders() {
	headers := []string{"NAME", "STATUS", "COMPLETIONS", "DURATION", "AGE", "PODS", "GPU", "GPU INFO"}
	for i, h := range headers {
		v.table.SetCell(0, i, tview.NewTableCell(h).SetSelectable(false).SetTextColor(tcell.ColorWhite).SetAlign(tview.AlignLeft))
	}
}

func (v *jobsView) renderStatus() {
	owner := "All"
	if v.onlyOwn {
		owner = "Mine"
	}
	prefix := ""
	if v.refreshing {
		prefix = "⟳ "
	}
	search := ""
	if v.searchTerm != "" {
		search = fmt.Sprintf(" | search:%q", v.searchTerm)
	}
	v.statusBar.SetText(fmt.Sprintf("%s(F)ilter: %s | (H)ide Others: %s | (S)ort: %s%s | press ? for help",
		prefix, v.filter.Label(), owner, v.sortMode.Label(), search))
}

func (v *jobsView) renderTable() {
	for r := v.table.GetRowCount() - 1; r > 0; r-- {
		v.table.RemoveRow(r)
	}
	jobs := append([]model.Job{}, v.jobs...)
	jobs = model.Filter(jobs, v.filter, v.app.user, v.onlyOwn)
	if v.searchTerm != "" {
		needle := strings.ToLower(v.searchTerm)
		filtered := jobs[:0:0]
		for _, j := range jobs {
			if strings.Contains(strings.ToLower(j.Name), needle) {
				filtered = append(filtered, j)
			}
		}
		jobs = filtered
	}
	model.Sort(jobs, v.sortMode)
	for i, j := range jobs {
		row := i + 1
		v.table.SetCell(row, 0, tview.NewTableCell(j.Name))
		v.table.SetCell(row, 1, tview.NewTableCell(string(j.Status)).SetTextColor(statusColor(j.Status)))
		v.table.SetCell(row, 2, tview.NewTableCell(j.CompletionsLabel()))
		v.table.SetCell(row, 3, tview.NewTableCell(j.Duration()))
		v.table.SetCell(row, 4, tview.NewTableCell(j.Age()))
		v.table.SetCell(row, 5, tview.NewTableCell(j.PodsLabel()))
		v.table.SetCell(row, 6, tview.NewTableCell(fmt.Sprintf("%d", j.GPUCount)).SetTextColor(gpuCountColor(j.GPUCount)))
		v.table.SetCell(row, 7, tview.NewTableCell(j.GPU.Label()).SetTextColor(gpuModelColor(j.GPU)))
	}
}

func (v *jobsView) scheduleRefresh() {
	if v.refreshing {
		return
	}
	if time.Since(v.lastRefresh) < refreshThrottle && !v.lastRefresh.IsZero() {
		return
	}
	v.refreshing = true
	v.renderStatus()
	go func() {
		ctx, cancel := context.WithTimeout(v.app.ctx, listTimeout)
		defer cancel()
		jobs, err := v.app.client.ListJobs(ctx)
		v.app.app.QueueUpdateDraw(func() {
			v.refreshing = false
			v.lastRefresh = time.Now()
			if err != nil {
				v.app.showError(fmt.Errorf("refresh: %w", err))
			} else {
				v.jobs = jobs
				v.renderTable()
			}
			v.renderStatus()
		})
	}()
}

func (v *jobsView) selectedJob() (string, string, bool) {
	row, _ := v.table.GetSelection()
	if row <= 0 {
		return "", "", false
	}
	name := v.table.GetCell(row, 0).Text
	status := v.table.GetCell(row, 1).Text
	return name, status, name != ""
}

func (v *jobsView) handleKey(ev *tcell.EventKey) *tcell.EventKey {
	if ev.Key() == tcell.KeyEscape {
		v.app.cancel()
		v.app.app.Stop()
		return nil
	}
	if ev.Key() != tcell.KeyRune {
		return ev
	}
	switch ev.Rune() {
	case 'q':
		v.app.cancel()
		v.app.app.Stop()
		return nil
	case 'r':
		v.scheduleRefresh()
		return nil
	case 'f':
		v.filter = v.filter.Next()
		v.renderTable()
		v.renderStatus()
		return nil
	case 'h':
		v.onlyOwn = !v.onlyOwn
		v.renderTable()
		v.renderStatus()
		return nil
	case 's':
		v.sortMode = v.sortMode.Next()
		v.renderTable()
		v.renderStatus()
		return nil
	case 'd':
		v.handleDelete()
		return nil
	case 'e':
		v.handleEnter()
		return nil
	case 'c':
		v.handleViewConfig()
		return nil
	case 'l':
		v.handleLogs()
		return nil
	case 'i':
		v.handleDescribe()
		return nil
	case '?':
		showHelp(v.app)
		return nil
	case '/':
		v.openSearch()
		return nil
	case 'n':
		showCreateForm(v.app, func() { v.scheduleRefresh() })
		return nil
	}
	return ev
}

// handleDelete shows a confirmation modal then deletes asynchronously.
func (v *jobsView) handleDelete() {
	name, status, ok := v.selectedJob()
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(v.app.ctx, listTimeout)
		defer cancel()
		j, err := v.app.client.GetJob(ctx, name)
		v.app.app.QueueUpdateDraw(func() {
			if err != nil {
				v.app.showError(err)
				return
			}
			owner := j.Labels[v.app.client.UserLabel()]
			if owner != v.app.user {
				v.app.showError(fmt.Errorf("cannot delete %s: owner is %q, you are %q", name, owner, v.app.user))
				return
			}
			labels := make([]string, 0, len(j.Labels))
			for k, val := range j.Labels {
				labels = append(labels, fmt.Sprintf("%s: %s", k, val))
			}
			text := fmt.Sprintf("⚠️  Delete job %q (status: %s)?\nLabels:\n%s", name, status, strings.Join(labels, "\n"))
			v.app.showModal(text, []string{"Cancel", "Confirm"}, func(label string) {
				if label != "Confirm" {
					return
				}
				go v.doDelete(name)
			})
		})
	}()
}

func (v *jobsView) doDelete(name string) {
	ctx, cancel := context.WithTimeout(v.app.ctx, listTimeout)
	defer cancel()
	err := v.app.client.DeleteJob(ctx, name)
	v.app.app.QueueUpdateDraw(func() {
		if err != nil {
			v.app.showError(err)
			return
		}
		klog.Action("delete", name)
		v.app.showMessage(fmt.Sprintf("Job %q deleted.", name))
		v.scheduleRefresh()
	})
}

// handleEnter execs into the running pod for a job.
func (v *jobsView) handleEnter() {
	name, status, ok := v.selectedJob()
	if !ok {
		return
	}
	if status != string(model.StatusRunning) {
		v.app.showError(fmt.Errorf("job %s is not running (status: %s)", name, status))
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(v.app.ctx, listTimeout)
		defer cancel()
		j, err := v.app.client.GetJob(ctx, name)
		if err != nil {
			v.app.app.QueueUpdateDraw(func() { v.app.showError(err) })
			return
		}
		owner := j.Labels[v.app.client.UserLabel()]
		if owner != v.app.user {
			v.app.app.QueueUpdateDraw(func() {
				v.app.showError(fmt.Errorf("cannot exec into %s: owner is %q", name, owner))
			})
			return
		}
		klog.Action("exec", name)
		v.execTTY(name)
	}()
}

func (v *jobsView) execTTY(name string) {
	var execErr error
	v.app.app.Suspend(func() {
		fmt.Print("\033[H\033[2J")
		ctx, cancel := context.WithCancel(v.app.ctx)
		defer cancel()
		execErr = v.app.client.ExecJob(ctx, name, os.Stdin, os.Stdout, os.Stderr, true)
		fmt.Print("\033[H\033[2J")
	})
	if execErr != nil {
		v.app.showError(fmt.Errorf("exec %s: %w", name, execErr))
	}
}

// openSearch reveals the search input and shifts focus into it. It does not
// clear any existing search term, so users can refine their previous query.
func (v *jobsView) openSearch() {
	v.root.ResizeItem(v.searchInput, 1, 0)
	v.searchInput.SetText(v.searchTerm)
	v.app.app.SetFocus(v.searchInput)
}

// closeSearch hides the search input row and returns focus to the table.
// The current searchTerm is preserved so the filter remains applied; users
// can press Esc again or '/' followed by Esc to clear it.
func (v *jobsView) closeSearch() {
	v.root.ResizeItem(v.searchInput, 0, 0)
	v.app.app.SetFocus(v.table)
	v.renderTable()
	v.renderStatus()
}

// handleLogs opens a scrollable view onto the job's pod logs.
func (v *jobsView) handleLogs() {
	name, _, ok := v.selectedJob()
	if !ok {
		return
	}
	klog.Action("logs", name)
	showLogs(v.app, name)
}

// handleDescribe opens a kubectl-describe-style view for the selected job.
func (v *jobsView) handleDescribe() {
	name, _, ok := v.selectedJob()
	if !ok {
		return
	}
	klog.Action("describe", name)
	showDescribe(v.app, name)
}

// handleViewConfig opens the job's manifest read-only in the user's editor.
func (v *jobsView) handleViewConfig() {
	name, _, ok := v.selectedJob()
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(v.app.ctx, listTimeout)
		defer cancel()
		yamlBytes, err := v.app.client.GetJobYAML(ctx, name)
		if err != nil {
			v.app.app.QueueUpdateDraw(func() { v.app.showError(err) })
			return
		}
		v.app.app.Suspend(func() {
			if _, err := editor.Edit(".yaml", yamlBytes, true); err != nil {
				fmt.Fprintf(os.Stderr, "editor error: %v\n", err)
			}
		})
	}()
}
