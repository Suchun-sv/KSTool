package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/suchun/kstool/internal/k8s"

	corev1 "k8s.io/api/core/v1"
)

const (
	describePage    = "describe"
	describeTimeout = 15 * time.Second
)

type describeView struct {
	app      *App
	jobName  string
	textView *tview.TextView
	header   *tview.TextView
	root     *tview.Flex
	pop      func()
}

func showDescribe(a *App, jobName string) {
	v := &describeView{app: a, jobName: jobName}
	v.header = tview.NewTextView().SetTextAlign(tview.AlignLeft)
	v.header.SetText("(r)efresh  (q)/Esc back  ↑/↓/PgUp/PgDn scroll")

	v.textView = tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetScrollable(true)
	v.textView.SetBorder(true).SetTitle(fmt.Sprintf(" describe · %s ", jobName)).SetTitleAlign(tview.AlignLeft)
	v.textView.SetInputCapture(v.handleKey)

	v.root = tview.NewFlex().SetDirection(tview.FlexRow)
	v.root.AddItem(v.header, 1, 0, false)
	v.root.AddItem(v.textView, 0, 1, true)

	v.pop = a.pushPage(describePage, v.root)
	go v.refresh()
}

func (v *describeView) handleKey(ev *tcell.EventKey) *tcell.EventKey {
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
		go v.refresh()
		return nil
	}
	return ev
}

func (v *describeView) close() {
	if v.pop != nil {
		v.pop()
		v.pop = nil
	}
}

func (v *describeView) refresh() {
	ctx, cancel := context.WithTimeout(v.app.ctx, describeTimeout)
	defer cancel()
	snap, err := v.app.client.DescribeJob(ctx, v.jobName)
	v.app.app.QueueUpdateDraw(func() {
		v.textView.Clear()
		if err != nil {
			fmt.Fprintf(v.textView, "[red]error: %v[-]\n", err)
			return
		}
		v.textView.SetText(formatDescribe(snap))
		v.textView.ScrollToBeginning()
	})
}

// formatDescribe renders a JobSnapshot as a human-readable text block. Output
// is intentionally similar to `kubectl describe job` but trimmed to the
// fields KSTool users typically care about.
func formatDescribe(s *k8s.JobSnapshot) string {
	var b strings.Builder
	j := s.Job
	fmt.Fprintf(&b, "[::b]Name:[-]      %s\n", j.Name)
	fmt.Fprintf(&b, "[::b]Namespace:[-] %s\n", j.Namespace)
	if len(j.Labels) > 0 {
		fmt.Fprintf(&b, "[::b]Labels:[-]\n")
		for k, val := range j.Labels {
			fmt.Fprintf(&b, "  %s=%s\n", k, val)
		}
	}
	fmt.Fprintf(&b, "[::b]Created:[-]   %s\n", j.CreationTimestamp.Format(time.RFC3339))
	if j.Status.StartTime != nil {
		fmt.Fprintf(&b, "[::b]Started:[-]   %s\n", j.Status.StartTime.Format(time.RFC3339))
	}
	if j.Status.CompletionTime != nil {
		fmt.Fprintf(&b, "[::b]Completed:[-] %s\n", j.Status.CompletionTime.Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "[::b]Counts:[-]    Active=%d Succeeded=%d Failed=%d\n",
		j.Status.Active, j.Status.Succeeded, j.Status.Failed)

	if len(j.Status.Conditions) > 0 {
		fmt.Fprintf(&b, "\n[::b]Conditions:[-]\n")
		for _, c := range j.Status.Conditions {
			ts := c.LastTransitionTime.Format(time.RFC3339)
			fmt.Fprintf(&b, "  %s=%s  %s  %s  %s\n", c.Type, c.Status, c.Reason, ts, c.Message)
		}
	}

	if len(s.Pods) == 0 {
		fmt.Fprintf(&b, "\n[::b]Pods:[-] (none)\n")
	} else {
		fmt.Fprintf(&b, "\n[::b]Pods:[-]\n")
		for i := range s.Pods {
			writePodSummary(&b, &s.Pods[i])
		}
	}

	if len(s.Events) == 0 {
		fmt.Fprintf(&b, "\n[::b]Events:[-] (none)\n")
	} else {
		fmt.Fprintf(&b, "\n[::b]Events:[-]\n")
		for _, ev := range s.Events {
			writeEvent(&b, ev)
		}
	}
	return b.String()
}

func writePodSummary(b *strings.Builder, p *corev1.Pod) {
	fmt.Fprintf(b, "  [::b]%s[-]  phase=%s  node=%s\n", p.Name, p.Status.Phase, p.Spec.NodeName)
	if p.Status.Reason != "" || p.Status.Message != "" {
		fmt.Fprintf(b, "    reason: %s  msg: %s\n", p.Status.Reason, p.Status.Message)
	}
	for _, cs := range p.Status.ContainerStatuses {
		state := containerStateString(cs.State)
		last := containerStateString(cs.LastTerminationState)
		fmt.Fprintf(b, "    container %s  ready=%t  restarts=%d\n", cs.Name, cs.Ready, cs.RestartCount)
		fmt.Fprintf(b, "      state: %s\n", state)
		if last != "(none)" {
			fmt.Fprintf(b, "      last:  %s\n", last)
		}
	}
}

func containerStateString(st corev1.ContainerState) string {
	switch {
	case st.Running != nil:
		return fmt.Sprintf("Running since %s", st.Running.StartedAt.Format(time.RFC3339))
	case st.Terminated != nil:
		t := st.Terminated
		return fmt.Sprintf("Terminated reason=%s exit=%d %s", t.Reason, t.ExitCode, t.Message)
	case st.Waiting != nil:
		return fmt.Sprintf("Waiting reason=%s %s", st.Waiting.Reason, st.Waiting.Message)
	}
	return "(none)"
}

func writeEvent(b *strings.Builder, ev corev1.Event) {
	ts := ev.LastTimestamp.Time
	if ts.IsZero() {
		ts = ev.EventTime.Time
	}
	tag := ev.Type
	color := ""
	switch tag {
	case corev1.EventTypeWarning:
		color = "[yellow]"
	case corev1.EventTypeNormal:
		color = "[green]"
	}
	reset := ""
	if color != "" {
		reset = "[-]"
	}
	fmt.Fprintf(b, "  %s  %s%s%s  %s/%s  %s\n",
		ts.Format(time.RFC3339), color, tag, reset,
		ev.InvolvedObject.Kind, ev.InvolvedObject.Name, ev.Message)
}
