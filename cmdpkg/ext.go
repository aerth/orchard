package cmdpkg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"m/pkg/global"
)

func Cheatcode(ctx context.Context, s string) error {
	sctx := global.NewCancel(ctx)
	for i := range 2 {
		slog.Warn("asking", "q", s)
		println()
		println()
		cmd := exec.CommandContext(sctx, "/usr/bin/gum", "confirm", "--default=no", "--padding=\"10 10\"", "--prompt.background=#FF0000", s)
		cmd.Stdin = os.Stdin
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		err := cmd.Run()
		println()
		println()
		if err != nil { // this catches
			//		slog.Error("error asking", "err", err)
			return err
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
