package cmdpkg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"

	"m/orchard/pkg/debugging"
	"m/orchard/pkg/global"
	"m/orchardtui/mainmodel"
	"m/orchardtui/tuilite"

	"github.com/aerth/mostly/stackerr"
)

var NumConfirm = 1

// Cheatcode plain confirmation (uses gum for now)
//
// in TUI code, this should be wrapped for proper display (ie: p.ReleaseTerminal/p.RestoreTerminal)
func Cheatcode(ctx context.Context, s string) error {
	sctx := global.NewCancel(ctx)
	for i := 1; i <= max(1, NumConfirm); i++ {
		slog.Warn("asking", "q", s)
		_, err := fmt.Fprintf(os.Stdout, "\r%s", debugging.ColorReset)
		if err != nil {
			return stackerr.Errorf("%w", err)
		}
		_, err = fmt.Fprintf(os.Stderr, "\r%s", debugging.ColorReset)
		if err != nil {
			return stackerr.Errorf("%w", err)
		}
		mo := tuilite.GetMainModel[mainmodel.Model]()
		println(mo.GetMM().V.GetContent())
		cmd := exec.CommandContext(sctx, "/usr/bin/gum", "confirm", "--default=no", "--padding=\"10 10\"", "--prompt.background=#FF0000", "--prompt.foreground=#FFFFFF", s)
		cmd.Stdin = os.Stdin
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		err = cmd.Run()
		if err != nil {
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
