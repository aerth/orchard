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

var noTemplate bool

func stringOrFile(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if strings.Count(s, " ") > 2 {
		return s, nil
	}
	b, err := os.ReadFile(s)
	if err != nil {
		return s, err
	}
	return string(b), nil
}

// GetSystemBase where data is [templatedata.NewData]
func GetSystemBase(data any) (string, error) {
	endingMessage, err := stringOrFile(Get().Get().SystemPrompt) // possibly a file
	if err != nil {
		return "", err
	}
	if !noTemplate {
		buf := strings.Builder{}
		tpl := template.New("").Funcs(mdloaders.FuncMap())
		var err error
		name := "SYSTEM.md"
		if strings.Count(endingMessage, " ") == 0 && endingMessage != "" {
			name = filepath.Base(endingMessage)
			slog.Info("loading system", "f", endingMessage)
			tpl, err = tpl.ParseFiles(endingMessage)
			if errors.Is(err, os.ErrNotExist) {
				slog.Error("system prompt file does not exist", "path", endingMessage)
			} else if err != nil {
				panic(err.Error()) // crash hard fast
			}
		} else if endingMessage != "" {
			name = ""
			tpl, err = tpl.Parse(endingMessage)
		} else {
			// embedded SYSTEM.md
			tpl, err = tpl.ParseFS(orchard.Embedded, "SYSTEM.md")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			panic("template SYSTEM.md failed: " + err.Error())
		}
		if err != nil {
			slog.Warn("NO SYSTEM.md FOUND", "err", err)
			return "You are a helpful assistant who speaks only nadsat hacker slang. Keep it simple and quick replies only.", err
		}
		if tpl == nil {
			panic("tpl is nil")
		}
		if err := tpl.ExecuteTemplate(&buf, name, data); err != nil {
			slog.Error("error executing template", "err", err)
		}
		endingMessage = buf.String()
	}
	pre := getPre(data)
	addsuffix := Get().Get().AddSystemPrompt
	return strings.Join([]string{pre, endingMessage, addsuffix}, "\n"), nil
}

func getPre(data any) string {
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
	return pre
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
