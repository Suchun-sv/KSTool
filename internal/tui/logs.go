package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	logsPage         = "job-logs"
	logsFetchTimeout = 30 * time.Second
)

type logsView struct {
	app     *App
	jobName string

	root        *tview.Flex
	header      *tview.TextView
	textView    *tview.TextView
	searchInput *tview.InputField

	mu         sync.Mutex
	rawLines   []string // every line ever shown, used to filter without refetching
	searchTerm string
	follow     bool
	stopFollow context.CancelFunc
	streamRC   io.Closer
	pop        func()
}

// showLogs opens a scrollable log view for jobName.
func showLogs(a *App, jobName string) {
	v := &logsView{app: a, jobName: jobName}

	v.header = tview.NewTextView().SetTextAlign(tview.AlignLeft)
	v.header.SetTextColor(tcell.ColorWhite)

	v.textView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(false).
		SetScrollable(true)
	v.textView.SetBorder(true).SetTitle(fmt.Sprintf(" logs · %s ", jobName)).SetTitleAlign(tview.AlignLeft)
	v.textView.SetInputCapture(v.handleKey)

	v.searchInput = tview.NewInputField().SetLabel("/ filter: ").SetFieldWidth(0)
	v.searchInput.SetChangedFunc(func(text string) {
		v.mu.Lock()
		v.searchTerm = text
		v.mu.Unlock()
		v.applyFilter()
	})
	v.searchInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			v.mu.Lock()
			v.searchTerm = ""
			v.mu.Unlock()
			v.searchInput.SetText("")
			v.applyFilter()
		}
		v.closeSearch()
	})

	v.root = tview.NewFlex().SetDirection(tview.FlexRow)
	v.root.AddItem(v.header, 1, 0, false)
	v.root.AddItem(v.searchInput, 0, 0, false)
	v.root.AddItem(v.textView, 0, 1, true)

	v.renderHeader()
	v.pop = a.pushPage(logsPage, v.root)
	go v.refreshSnapshot()
}

func (v *logsView) renderHeader() {
	mode := "snapshot"
	if v.follow {
		mode = "following"
	}
	filt := ""
	if v.searchTerm != "" {
		filt = fmt.Sprintf("  filter:%q", v.searchTerm)
	}
	v.header.SetText(fmt.Sprintf("[%s]%s  (r)efresh  (f)ollow  (/)filter  (q)/Esc back", mode, filt))
}

func (v *logsView) handleKey(ev *tcell.EventKey) *tcell.EventKey {
	if ev.Key() == tcell.KeyEscape {
		v.close()
		return nil
	}
	if ev.Key() != tcell.KeyRune {
		return ev
	}
	switch ev.Rune() {
	case 'q':
		v.close()
		return nil
	case 'r':
		v.stopStream()
		v.mu.Lock()
		v.rawLines = v.rawLines[:0]
		v.mu.Unlock()
		v.textView.Clear()
		go v.refreshSnapshot()
		return nil
	case 'f':
		v.toggleFollow()
		return nil
	case '/':
		v.openSearch()
		return nil
	}
	return ev
}

func (v *logsView) openSearch() {
	v.root.ResizeItem(v.searchInput, 1, 0)
	v.searchInput.SetText(v.searchTerm)
	v.app.app.SetFocus(v.searchInput)
}

func (v *logsView) closeSearch() {
	v.root.ResizeItem(v.searchInput, 0, 0)
	v.app.app.SetFocus(v.textView)
	v.renderHeader()
}

func (v *logsView) close() {
	v.stopStream()
	if v.pop != nil {
		v.pop()
		v.pop = nil
	}
}

// applyFilter rewrites textView from rawLines, keeping only matching lines.
func (v *logsView) applyFilter() {
	v.mu.Lock()
	term := strings.ToLower(v.searchTerm)
	lines := make([]string, 0, len(v.rawLines))
	if term == "" {
		lines = append(lines, v.rawLines...)
	} else {
		for _, ln := range v.rawLines {
			if strings.Contains(strings.ToLower(ln), term) {
				lines = append(lines, ln)
			}
		}
	}
	v.mu.Unlock()
	v.app.app.QueueUpdateDraw(func() {
		v.textView.Clear()
		fmt.Fprint(v.textView, strings.Join(lines, "\n"))
		if len(lines) > 0 {
			v.textView.ScrollToEnd()
		}
		v.renderHeader()
	})
}

// appendLine adds a line to the raw buffer and writes it to the view if it
// passes the active filter.
func (v *logsView) appendLine(line string) {
	v.mu.Lock()
	v.rawLines = append(v.rawLines, line)
	matches := v.searchTerm == "" || strings.Contains(strings.ToLower(line), strings.ToLower(v.searchTerm))
	v.mu.Unlock()
	if matches {
		v.app.app.QueueUpdateDraw(func() {
			fmt.Fprintln(v.textView, line)
			v.textView.ScrollToEnd()
		})
	}
}

// refreshSnapshot fetches a one-shot tail of the pod logs and replaces the
// view contents.
func (v *logsView) refreshSnapshot() {
	ctx, cancel := context.WithTimeout(v.app.ctx, logsFetchTimeout)
	defer cancel()
	data, err := v.app.client.JobLogs(ctx, v.jobName, v.app.cfg.LogsTailLines)
	if err != nil {
		v.app.app.QueueUpdateDraw(func() {
			fmt.Fprintf(v.textView, "[error] %v\n", err)
		})
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	v.mu.Lock()
	v.rawLines = append(v.rawLines[:0], lines...)
	v.mu.Unlock()
	v.applyFilter()
}

// toggleFollow flips follow mode.
func (v *logsView) toggleFollow() {
	v.mu.Lock()
	if v.follow {
		v.mu.Unlock()
		v.stopStream()
		v.mu.Lock()
		v.follow = false
		v.mu.Unlock()
		v.renderHeader()
		return
	}
	v.follow = true
	v.mu.Unlock()
	v.renderHeader()
	go v.startStream()
}

func (v *logsView) startStream() {
	ctx, cancel := context.WithCancel(v.app.ctx)
	v.mu.Lock()
	v.stopFollow = cancel
	v.mu.Unlock()

	rc, err := v.app.client.StreamJobLogs(ctx, v.jobName, v.app.cfg.LogsTailLines)
	if err != nil {
		v.app.app.QueueUpdateDraw(func() {
			fmt.Fprintf(v.textView, "\n[stream error] %v\n", err)
		})
		v.mu.Lock()
		v.follow = false
		v.stopFollow = nil
		v.mu.Unlock()
		v.renderHeader()
		return
	}
	v.mu.Lock()
	v.streamRC = rc
	v.mu.Unlock()

	scanner := bufio.NewScanner(rc)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		v.appendLine(scanner.Text())
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		v.app.app.QueueUpdateDraw(func() {
			fmt.Fprintf(v.textView, "\n[stream ended] %v\n", err)
		})
	}
	v.mu.Lock()
	v.follow = false
	v.streamRC = nil
	v.stopFollow = nil
	v.mu.Unlock()
	v.app.app.QueueUpdateDraw(v.renderHeader)
}

func (v *logsView) stopStream() {
	v.mu.Lock()
	cancel := v.stopFollow
	rc := v.streamRC
	v.stopFollow = nil
	v.streamRC = nil
	v.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if rc != nil {
		_ = rc.Close()
	}
}
