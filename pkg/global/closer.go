// Package global readonly things
package global

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

func AddCloser(closer func() error) {
	closers = append(closers, closer)
}

var closers []func() error

func Close() {
	if len(closers) == 0 && len(cancels) == 0 {
		return
	}
	Cancel(fmt.Errorf("shutdown"))
	cl := closers
	closers = nil
	for i := range cl {
		if err := cl[i](); err != nil {
			slog.Error("closing error", "n", i, "err", err)
		}
	}
}

var cancels []context.CancelCauseFunc

func NewCancel(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancelCause(ctx)
	AddCancel(cancel)
	return ctx
}

func AddCancel(c context.CancelCauseFunc) {
	shouldClose = 0
	cancels = append(cancels, c)
}

var shouldClose = 0

func Cancel(err error) {
	x := cancels
	cancels = nil
	if len(x) == 0 {
		shouldClose++
		if shouldClose > 3 {
			Close()
			os.Exit(1)
		}
		return
	}
	shouldClose = 0
	for i := range x {
		x[i](err)
	}
}
