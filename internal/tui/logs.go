package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	logsPage         = "job-logs"
	logsTailLines    = 2000
	logsFetchTimeout = 30 * time.Second
)

type logsView struct {
	app     *App
	jobName string

	root      *tview.Flex
	header    *tview.TextView
	textView  *tview.TextView

	mu        sync.Mutex
	follow    bool
	stopFollow context.CancelFunc
	streamRC  io.Closer
	pop       func()
}

// showLogs opens a scrollable log view for jobName.
func showLogs(a *App, jobName string) {
	v := &logsView{app: a, jobName: jobName}

	v.header = tview.NewTextView().SetTextAlign(tview.AlignLeft)
	v.header.SetTextColor(tcell.ColorWhite)

	v.textView = tview.NewTextView().
		SetDynamicColors(false).
		SetWrap(false).
		SetScrollable(true).
		SetChangedFunc(func() { a.app.Draw() })
	v.textView.SetBorder(true).SetTitle(fmt.Sprintf(" logs · %s ", jobName)).SetTitleAlign(tview.AlignLeft)

	v.textView.SetInputCapture(v.handleKey)

	v.root = tview.NewFlex().SetDirection(tview.FlexRow)
	v.root.AddItem(v.header, 1, 0, false)
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
	v.header.SetText(fmt.Sprintf("[%s]  (r)efresh  (f)ollow toggle  (q)/Esc back  ↑/↓/PgUp/PgDn scroll", mode))
}

func (v *logsView) handleKey(ev *tcell.EventKey) *tcell.EventKey {
	if ev.Key() == tcell.KeyEscape {
		v.close()
		return nil
	}
	if ev.Key() != tcell.KeyRune {
		return ev // let the textview handle scrolling
	}
	switch ev.Rune() {
	case 'q':
		v.close()
		return nil
	case 'r':
		v.stopStream()
		v.textView.Clear()
		go v.refreshSnapshot()
		return nil
	case 'f':
		v.toggleFollow()
		return nil
	}
	return ev
}

func (v *logsView) close() {
	v.stopStream()
	if v.pop != nil {
		v.pop()
		v.pop = nil
	}
}

// refreshSnapshot fetches a one-shot tail of the pod logs and replaces the
// view contents. Runs on its own goroutine.
func (v *logsView) refreshSnapshot() {
	ctx, cancel := context.WithTimeout(v.app.ctx, logsFetchTimeout)
	defer cancel()
	data, err := v.app.client.JobLogs(ctx, v.jobName, logsTailLines)
	v.app.app.QueueUpdateDraw(func() {
		if err != nil {
			fmt.Fprintf(v.textView, "[error] %v\n", err)
			return
		}
		v.textView.SetText(string(data))
		v.textView.ScrollToEnd()
	})
}

// toggleFollow flips follow mode. Off→on opens a streaming reader and pumps
// lines into the text view. On→off cancels the stream.
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

	rc, err := v.app.client.StreamJobLogs(ctx, v.jobName, logsTailLines)
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
		line := scanner.Text()
		v.app.app.QueueUpdateDraw(func() {
			fmt.Fprintln(v.textView, line)
			v.textView.ScrollToEnd()
		})
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
