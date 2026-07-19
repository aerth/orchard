package global

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"text/template"
	"time"
)

// NewData for now
func NewData() map[string]any {
	return map[string]any{"OrchardVersion": GetVersion(), "Now": time.Now()}
}

func GetSystemBase(data any) string {
	tpl, err := template.New("").ParseFiles("SYSTEM2.md")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		panic("template SYSTEM.md failed: " + err.Error())
	}
	if err != nil {
		return "You are a helpful assistant who speaks only nadsat hacker slang. Keep it simple and quick replies only."
	}
	var buf strings.Builder
	pre := ""
	if err := tpl.ExecuteTemplate(&buf, "SYSTEM.md", NewData()); err != nil {
		slog.Error("error executing template", "err", err)
	}
	return strings.Join([]string{buf.String(), pre}, "\n")
}
