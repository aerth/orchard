package global

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"m/pkg/loaders/mdloaders"

	orchard "m"
)

// GetSystemBase where data is [templatedata.NewData]
func GetSystemBase(data any) string {
	endingMessage := []string{Get().Get().SystemPrompt}

	tpl := template.New("").Funcs(mdloaders.FuncMap())
	var err error
	name := "SYSTEM.md"
	if len(endingMessage) == 1 && endingMessage[0] != "" {
		name = filepath.Base(endingMessage[0])
		slog.Info("loading system", "f", endingMessage[0])
		tpl, err = tpl.ParseFiles(endingMessage...)
		if errors.Is(err, os.ErrNotExist) {
			slog.Error("system prompt file does not exist", "path", endingMessage[0])
		} else if err != nil {
			panic(err.Error()) // crash hard fast
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
		pre = string("## Project-level ORCHARD.md file contents\n" + QuickTemplate(string(orchardfile), data))
	}
	if errors.Is(err, os.ErrNotExist) {
		orchardfile, err = os.ReadFile("AGENTS.md")
		if err == nil {
			pre = string("## Project-level AGENTS.md file contents\n" + QuickTemplate(string(orchardfile), data))
		}
	}
	got := strings.TrimSpace(strings.Join([]string{buf.String(), pre}, "\n"))
	slog.Info("sticky system instruction", "size", len(got))
	withEnding := strings.Join(append([]string{Get().Get().AddSystemPrompt, got}, endingMessage...), "\n")
	if !printfirst1 {
		slog.Info("sticky:\n" + withEnding)
		printfirst1 = true
	}
	//_, _ = fmt.Fprintf(log.Writer(), "%s\n", got)
	return withEnding
}

var printfirst1 = testing.Testing()

// QuickTemplate best-effort template parse and execute, otherwise return plain s
func QuickTemplate(s string, data any) string {
	if s == "" {
		return ""
	}
	tmpl, err := template.New("quick").Parse(s)
	if err != nil {
		return s
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return s
	}

	return buf.String()
}
