package implv2

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"m/pkg/chat"
	"m/pkg/types/chatmessages"

	"github.com/aerth/mostly/stackerr"
	"github.com/google/uuid"
)

type PreReview struct {
	Filename string
	History  chatmessages.A
	Work     chatmessages.A
	Before   []string
	After    []string
}

func DoReview(ctx context.Context, info PreReview, e chat.Ext) (string, error) {
	reviewID := uuid.New()
	return "", fmt.Errorf("review not implemented here yet")
}

// Differ error may occur because of filesystem or running diff
func Differ(id uuid.UUID, before, after string) (string, error) {
	dir, err := os.MkdirTemp("", id.String())
	if err != nil {
		return "", stackerr.Errorf("%w", err)
	}
	file1 := filepath.Join(dir, "A.txt")
	err = os.WriteFile(file1, []byte(before), 0o600)
	if err != nil {
		return "", stackerr.Errorf("%w", err)
	}
	defer func() {
		if err := os.Remove(file1); err != nil {
			slog.Error("could not remove temp file", "path", file1, "err", err)
		}
	}()
	file2 := filepath.Join(dir, "B.txt")
	err = os.WriteFile(file2, []byte(after), 0o600)
	if err != nil {
		return "", stackerr.Errorf("%w", err)
	}
	defer func() {
		if err := os.Remove(file1); err != nil {
			slog.Error("could not remove temp file", "path", file1, "err", err)
		}
	}()
	cmd := exec.Command("diff", file1, file2)
	out, err := cmd.CombinedOutput()
	err = errget(err, 1)
	return string(out), errget(err, 1)
}

func errget(err error, want int) error {
	if err == nil {
		return fmt.Errorf("expected exit code %d, got 0", want)
	}
	exitError, ok := err.(*exec.ExitError)
	if ok && exitError.ExitCode() == want {
		return nil
	}
	return err
}
