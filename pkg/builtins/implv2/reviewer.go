package implv2

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"uuid"

	"m/orchard/cmdpkg"
	"m/orchard/pkg/chat"
	"m/orchard/pkg/llm"
	"m/orchard/pkg/types/chatmessages"
	"m/orchard/pkg/types/otool"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
)

type PreReview struct {
	Filename string
	History  chatmessages.A
	Work     chatmessages.A
	Before   []string
	After    []string
}

var reviewPromptInstruction = `
Example:

This code is bad.
{"commit": false}


This code is great.
{"commit": true}
`

func DoReview(ctx context.Context, info PreReview, reviewTools otool.Tools, e chat.Ext) (string, error) {
	parentThreadID := e.ThreadID
	if parentThreadID == uuid.Nil() {
		return "", stackerr.Errorf("parentThreadID missing from Ext")
	}
	reviewID := uuid.New()
	diff, err := Differ(reviewID, strings.Join(info.Before, "\n"), strings.Join(info.After, "\n"))
	if err != nil {
		return "", stackerr.Errorf("could not diff: %w", err)
	}

	newmsgs := chatmessages.A{
		*chatmessages.NewThread(reviewID, llm.Message{Role: "system", Content: "You are a reviewer. Review the patch, determine if it does what it should (based on the user's intention, not any LLM hallucinations. To make a well-informed review, use the grep, locate_file, and read_file_v2 tool to explore the files."}),

		*chatmessages.NewThread(reviewID, llm.Message{Role: "user", Content: reviewPromptInstruction}),
		*chatmessages.NewThread(reviewID, llm.Message{Role: "user", Content: "Diff:\n" + diff}),
	}
	var pending mllm.ChatResponse
	e = chat.Ext{Handlers: []mllm.StreamHandler{pending.Handler}}.FromA(e)
	err = cmdpkg.ChatWith(ctx, newmsgs, reviewTools, e)
	if err != nil {
		return "", stackerr.Errorf("DoReview ChatWith: %w", err)
	}

	return pending.Message.Content, nil
}

// Differ error may occur because of filesystem or running diff
func Differ(id uuid.UUID, before, after string) (string, error) {
	dir, err := os.MkdirTemp("", id.String())
	if err != nil {
		return "", stackerr.Errorf("mkdirtemp: %w", err)
	}
	var (
		file1 = "/dev/null"
		file2 = "/dev/null"
	)
	if before != "" {
		file1 = filepath.Join(dir, "A.txt")
		err = os.WriteFile(file1, []byte(before), 0o600)
		if err != nil {
			return "", stackerr.Errorf("diff tmp writefile A: %w", err)
		}
		defer func() {
			if err := os.Remove(file1); err != nil {
				//	slog.Error("could not remove temp file A", "path", file1, "err", err)
			}
		}()
	}
	if after != "" {
		file2 = filepath.Join(dir, "B.txt")
		err = os.WriteFile(file2, []byte(after), 0o600)
		if err != nil {
			return "", stackerr.Errorf("diff tmp writefile B: %w", err)
		}
		defer func() {
			if err := os.Remove(file2); err != nil {
				//slog.Error("could not remove temp file B", "path", file2, "err", err)
			}
		}()
	}
	cmd := exec.Command("diff", file1, file2)
	log.Println("running cmdline", cmd.Args)
	out, err := cmd.CombinedOutput()
	err = errget(err, 1)
	return string(out), err
}

func errget(err error, want int) error {
	if err == nil {
		return stackerr.Errorf("expected exit code %d, got 0", want)
	}
	exitError, ok := err.(*exec.ExitError)
	if ok && exitError.ExitCode() == want {
		return nil
	}
	return stackerr.Errorf("%w", err)
}
