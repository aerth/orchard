package editv3

import (
	"context"
	"strings"
	"testing"

	"github.com/aerth/mostly/mllm"
)

type mockE struct {
	mllm.E
	responses []string
	turn      int
}

func TestEditorSession_EditFlow(t *testing.T) {
	mock := &mockE{
		responses: []string{
			"GOTO 2",
			"LOCK 2",
			"line 2 modified\nSAVE",
			"EXIT",
		},
	}

	_ = mock
	initial := []string{"line 1", "line 2", "line 3"}
	session := NewSession(mllm.E{}, "test.txt", initial)
	if err := session.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "line 1\nline 2 modified\nline 3"
	got := strings.Join(session.Draft, "\n")
	if got != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, got)
	}
}

func TestEditorSession_PrependFlow(t *testing.T) {
	mock := &mockE{
		responses: []string{
			"LOCK 0",
			"header line\nSAVE",
			"EXIT",
		},
	}

	_ = mock
	initial := []string{"line 1", "line 2"}
	session := NewSession(mllm.E{}, "test.txt", initial)

	if err := session.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "header line\nline 1\nline 2"
	got := strings.Join(session.Draft, "\n")
	if got != expected {
		t.Errorf("expected:\n%s\n\ngot:\n%s", expected, got)
	}
}

func TestEditorSession_InvalidLock(t *testing.T) {
	mock := &mockE{
		responses: []string{
			"LOCK 10-20", // Out of bounds
			"EXIT",
		},
	}

	_ = mock
	initial := []string{"line 1"}
	session := NewSession(mllm.Default(), "test.txt", initial)

	if err := session.Run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should recover from error and leave draft untouched
	if len(session.Draft) != 1 || session.Draft[0] != "line 1" {
		t.Errorf("draft modified on invalid lock: %v", session.Draft)
	}
}
