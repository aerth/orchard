package global

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	orchard "m"

	"m/pkg/loaders/mdloaders"
)

func GetSystemBase(data any) string {
	var s string
	if true {
		s = Get().Get().SystemPrompt
	}
	tpl := template.New("").Funcs(mdloaders.FuncMap())
	var err error
	name := "SYSTEM.md"
	if s != "" {
		name = filepath.Base(s)
		slog.Info("loading system", "f", s)
		tpl, err = tpl.ParseFiles(s)
		if errors.Is(err, os.ErrNotExist) {
			panic(err.Error())
		}
		if err != nil {
			panic(err.Error())
		}
	} else {
		tpl, err = tpl.ParseFS(orchard.Embedded, "SYSTEM.md")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		panic("template SYSTEM.md failed: " + err.Error())
	}
	if err != nil {
		slog.Warn("NO SYSTEM.md FOUND")
		return "You are a helpful assistant who speaks only nadsat hacker slang. Keep it simple and quick replies only."
	}
	if tpl == nil {
		panic("tpl is nil")
	}
	var buf strings.Builder
	if err := tpl.ExecuteTemplate(&buf, name, data); err != nil {
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
