package implv2

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"m/cmdpkg"
	"m/pkg/chat"
	"m/pkg/llm"
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
	parentThreadID := e.ThreadID
	if parentThreadID == uuid.Nil {
		return "", stackerr.Errorf("parentThreadID missing from Ext")
	}
	reviewID := uuid.New()
	diff, err := Differ(reviewID, strings.Join(info.Before, "\n"), strings.Join(info.After, "\n"))
	if err != nil {
		return "", stackerr.Errorf("could not diff: %w", err)
	}

	newmsgs := chatmessages.A{
		*chatmessages.NewThread(reviewID, llm.Message{Role: "system", Content: "You are a reviewer. Review the patch, determine if it does what it should."}),
		*chatmessages.NewThread(reviewID, llm.Message{Role: "user", Content: diff}),
	}
	got := *chatmessages.NewThread(parentThreadID, llm.Message{Role: "assistant"})
	e = chat.Ext{Handlers: []chat.ChatHandler{got.GatherChatResponse}}.FromA(e)
	err = cmdpkg.ChatWith(ctx, newmsgs, nil, e)
	if err != nil {
		return "", stackerr.Errorf("%w", err)
	}
	return got.Content, nil
}

// Differ error may occur because of filesystem or running diff
func Differ(id uuid.UUID, before, after string) (string, error) {
	dir, err := os.MkdirTemp("", id.String())
	if err != nil {
		return "", stackerr.Errorf("%w", err)
	}
	var (
		file1 = "/dev/null"
		file2 = "/dev/null"
	)
	if before != "" {
		file1 = filepath.Join(dir, "A.txt")
		err = os.WriteFile(file1, []byte(before), 0o600)
		if err != nil {
			return "", stackerr.Errorf("%w", err)
		}
		defer func() {
			if err := os.Remove(file1); err != nil {
				slog.Error("could not remove temp file", "path", file1, "err", err)
			}
		}()
	}
	if after != "" {
		file2 = filepath.Join(dir, "B.txt")
		err = os.WriteFile(file2, []byte(after), 0o600)
		if err != nil {
			return "", stackerr.Errorf("%w", err)
		}
		defer func() {
			if err := os.Remove(file1); err != nil {
				slog.Error("could not remove temp file", "path", file1, "err", err)
			}
		}()
	}
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
