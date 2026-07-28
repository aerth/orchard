package global

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"text/template"

	orchard "m"

	"m/pkg/loaders/mdloaders"
)

func GetSystemBase(data any) string {
	if s := Get().Get().SystemPrompt; s != "" {
		if strings.Contains(s, " ") {
			return s
		}
		b, err := os.ReadFile(s)
		return s
	}
	tpl, err := template.New("").Funcs(mdloaders.FuncMap()).ParseFS(orchard.Embedded, "SYSTEM.md")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		panic("template SYSTEM.md failed: " + err.Error())
	}
	if err != nil {
		return "You are a helpful assistant who speaks only nadsat hacker slang. Keep it simple and quick replies only."
	}
	var buf strings.Builder
	if err := tpl.ExecuteTemplate(&buf, "SYSTEM.md", data); err != nil {
		slog.Error("error executing template", "err", err)
	}
	pre := ""
	orchardfile, err := os.ReadFile("ORCHARD.md")
	if err == nil {
		pre = string("## Project-level ORCHARD.md file contents\n" + string(orchardfile))
	}
	if errors.Is(err, os.ErrNotExist) {
		orchardfile, err = os.ReadFile("AGENTS.md")
		if err == nil {
			pre = string("## Project-level AGENTS.md file contents\n" + string(orchardfile))
		}
	}
	got := strings.TrimSpace(strings.Join([]string{buf.String(), pre}, "\n"))
	slog.Info("sticky system instruction", "size", len(got))
	//_, _ = fmt.Fprintf(log.Writer(), "%s\n", got)
	return got
}
