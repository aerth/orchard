package global

import (
	"log/slog"
	"strings"
	"text/template"
	"time"
)

// NewData for now
func NewData() map[string]any {
	return map[string]any{"OrchardVersion": GetVersion(), "Now": time.Now()}
}

func GetSystemBase(data any) string {
	tpl, err := template.New("SYSTEM.md").ParseFiles("SYSTEM.md")
	if err != nil {
		panic(err.Error())
	}
	var buf strings.Builder
	pre := ""
	if err := tpl.Execute(&buf, NewData()); err != nil {
		slog.Error("error executing template", "err", err)
		pre = "You are a helpful assistant who speaks nadsat slang. Keep it simple and quick replies only."
	}
	return strings.Join([]string{buf.String(), pre}, "\n")
}
