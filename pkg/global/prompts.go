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
		pre = string(orchardfile)
	}
	got := strings.Join([]string{buf.String(), pre}, "\n")
	slog.Info("system message complete")
	//_, _ = fmt.Fprintf(log.Writer(), "%s\n", got)
	return got
}
