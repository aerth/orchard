package orchard

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"
)

//go:embed prompts/system.v1.md
var systemprompttemplate string

// SystemPrompt implements [SystemPrompter] and is used if no [Config.SystemPromptFunc].
func (cfg Config) SystemPrompt() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		slog.Error("getwd", "err", err)
		wd = fmt.Sprintf("Error getting working directory: %v", err) // give this one to llm too
	}
	s, err := executeTemplateString(systemprompttemplate, templateData{
		WorkingDirectory: wd,
		CurrentTime:      time.Now().Truncate(time.Second).UTC().Format(time.DateTime),
		Tools:            unstable{}.Names(cfg.Toolbox),
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

type templateData struct {
	WorkingDirectory string
	CurrentTime      string
	Tools            []string // for HasTool
	Extra            string
}

func (t templateData) HasTool(name string) bool {
	return slices.Contains(t.Tools, name)
}

func executeTemplateString(s string, d templateData) (string, error) {
	var buf bytes.Buffer
	t := template.Must(template.New("").Parse(s))
	if err := t.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}
