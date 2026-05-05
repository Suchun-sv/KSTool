package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const helpPage = "help"

const helpText = `[::b]Job list[-]
  ↑/↓ /j/k    move selection
  /           search by job name
  r           refresh now
  f           cycle status filter (All / Running / Failed / Pending)
  h           toggle "only my jobs"
  s           cycle sort
  d           delete selected job (owner-checked)
  e           exec into the running pod
  l           open log viewer
  i           describe (status + events)
  c           view manifest in $EDITOR (read-only)
  n           open create-job flow
  ?           this help
  q / Esc     quit

[::b]Log viewer[-]
  r           re-snapshot
  f           toggle follow mode
  /           filter lines by substring
  n / N       jump to next / previous match
  ↑/↓ PgUp/Dn scroll
  q / Esc     back

[::b]Describe viewer[-]
  r           refresh
  q / Esc     back

[::b]Create flow[-]
  Tab/Shift-Tab  move between fields
  e (on button)  edit YAML in $EDITOR
  Save / Apply   apply runs server-side dry-run first
  Esc            back (prompts if unsaved)`

// showHelp pushes a scrollable help modal over the current page.
func showHelp(a *App) {
	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetScrollable(true).
		SetText(helpText)
	tv.SetBorder(true).SetTitle(" KSTool — keys ").SetTitleAlign(tview.AlignLeft)

	var pop func()
	tv.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEscape || (ev.Key() == tcell.KeyRune && (ev.Rune() == 'q' || ev.Rune() == '?')) {
			pop()
			return nil
		}
		return ev
	})
	pop = a.pushPage(helpPage, tv)
}
