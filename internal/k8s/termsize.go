package k8s

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/term"
	"k8s.io/client-go/tools/remotecommand"
)

// terminalSizeQueue forwards local TTY resize events to the remote process.
// It implements remotecommand.TerminalSizeQueue.
//
// All sends to the channel happen on a single goroutine which closes the
// channel on exit. stop() just cancels the context, which lets the goroutine
// drain cleanly with no risk of sending on a closed channel.
type terminalSizeQueue struct {
	ch     chan remotecommand.TerminalSize
	cancel context.CancelFunc
	once   sync.Once
}

func newTerminalSizeQueue(parent context.Context) *terminalSizeQueue {
	ctx, cancel := context.WithCancel(parent)
	q := &terminalSizeQueue{
		ch:     make(chan remotecommand.TerminalSize, 4),
		cancel: cancel,
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGWINCH)

	push := func() {
		w, h, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil {
			return
		}
		select {
		case q.ch <- remotecommand.TerminalSize{Width: uint16(w), Height: uint16(h)}:
		default:
		}
	}

	go func() {
		defer close(q.ch)
		defer signal.Stop(sigs)
		push() // seed initial size
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigs:
				push()
			}
		}
	}()
	return q
}

// Next blocks until the next resize event or the queue is stopped.
func (q *terminalSizeQueue) Next() *remotecommand.TerminalSize {
	s, ok := <-q.ch
	if !ok {
		return nil
	}
	return &s
}

func (q *terminalSizeQueue) stop() {
	q.once.Do(q.cancel)
}
