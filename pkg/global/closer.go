// Package global readonly things
package global

import (
	"context"
	"fmt"
	"log/slog"
)

func AddCloser(closer func() error) {
	closers = append(closers, closer)
}

var closers []func() error

func Close() {
	Cancel(fmt.Errorf("shutdown"))
	for i := range closers {
		if err := closers[i](); err != nil {
			slog.Error("closing error", "n", i, "err", err)
		}
	}
}

var cancels []context.CancelCauseFunc

func AddCancel(c context.CancelCauseFunc) {
	cancels = append(cancels, c)
}

func Cancel(err error) {
	x := cancels
	cancels = nil
	for i := range x {
		x[i](err)
	}
}
