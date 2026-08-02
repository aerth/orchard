package cmdpkg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"m/pkg/debugging"
	"m/pkg/global"

	"github.com/aerth/mostly/stackerr"
)

var NumConfirm = 1

func Cheatcode(ctx context.Context, s string) error {
	sctx := global.NewCancel(ctx)
	for i := 1; i <= max(1, NumConfirm); i++ {
		slog.Warn("asking", "q", s)
		fmt.Fprintf(os.Stdout, "\r%s", debugging.ColorReset)
		fmt.Fprintf(os.Stderr, "\r%s", debugging.ColorReset)
		cmd := exec.CommandContext(sctx, "/usr/bin/gum", "confirm", "--default=no", "--padding=\"10 10\"", "--prompt.background=#FF0000", s)
		cmd.Stdin = os.Stdin
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		err := cmd.Run()
		println()
		println()
		if err != nil { // this catches
			//		slog.Error("error asking", "err", err)
			return stackerr.Errorf("%w", err)
		}
		if !cmd.ProcessState.Success() {
			return fmt.Errorf("nonzero exit")
		}
		if i == 0 {
			println("One more to go")
		}
	}
	return nil
}
