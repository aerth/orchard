// Package global for now
package global

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
)

func AddCloser(closer func() error) {
	closers = append(closers, closer)
}

var closers []func() error

var LogWriter io.Writer = os.Stderr

func Close() {
	if len(closers) == 0 && len(cancels) == 0 {
		return
	}
	cl := closers
	closers = nil
	CancelAll(fmt.Errorf("shutdown"))
	x := len(cl) - 1
	for i := range cl {
		if err := cl[x-i](); err != nil {
			slog.Error("closing error", "n", i, "err", err)
		}
	}
}

func GetTools[T any]() []T {
	if tools == nil {
		return nil
	}
	x, ok := tools.([]T)
	if !ok && tools != nil {
		slog.Warn("invalid type given", "want", fmt.Sprintf("%T", []T{}, "got", fmt.Sprintf("%T", tools)))
	}
	return x
}

func SetConf(conf Conf) {
	gconf = conf
}

var gconf Conf

func Get() Conf {
	return gconf
}

func SetTools[T any](v []T) {
	tools = v
}

var (
	tools   any
	cancels []context.CancelCauseFunc
)

func NewCancel(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	AddCancel(cancel)
	return ctx
}

func AddCancel(c context.CancelCauseFunc) {
	shouldClose = 0
	cancels = append(cancels, c)
}

func Reset() int {
	x := MAX - shouldClose
	shouldClose = 0
	return x
}

func Remaining() int {
	return MAX - shouldClose
}

var shouldClose = 0

const MAX = 4

func CancelAll(err error) {
	x := cancels
	cancels = nil
	for i := range x {
		x[i](err)
	}
}

var NotifyClosing = func(remaining int) {
	slog.Info("Press Ctrl+C again to close", "remaining", remaining)
}

func CancelCtrlC(err error) {
	if len(cancels) == 0 {
		shouldClose++
		NotifyClosing(MAX - shouldClose)
		if shouldClose >= MAX {
			slog.Error("max cancels, shutting down", "cause", err)
			Close()
		}
		return
	}
	shouldClose = 0
	CancelAll(err)
}
