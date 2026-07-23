package templatedata

import (
	"os"
	"os/exec"
	"time"

	"m/pkg/chatmessages"
	"m/pkg/global"

	"github.com/aerth/mostly/stackerr"
)

// NewData for now
func NewData(recentMessages chatmessages.ChatMessages) map[string]any {
	l := len(recentMessages)
	var recentMessage chatmessages.ChatMessage
	if l != 0 {
		recentMessage = recentMessages[l-1]
	}
	if l > 4 {
		recentMessage = recentMessages[l-3]
	}
	if len(cleanenv) == 0 {
		cleanenv = global.NewCleanEnviron()
	}
	if uname == "" {
		var err error
		uname, err = runUname()
		if err != nil {
			global.Cancel(stackerr.Errorf("%w", err))
		}
	}
	fastfetch, _ := runFastfetch()
	repoinfo, _ := runBashInfo()
	return map[string]any{
		"OrchardVersion": global.GetVersion(), "Now": time.Now(),
		"Model":         os.Getenv("OLLAMA_MODEL"),
		"Think":         os.Getenv("OLLAMA_THINK"),
		"uname":         uname,
		"Env":           cleanenv,
		"FastFetch":     fastfetch,
		"RecentMessage": recentMessage,
		"Messages":      recentMessages,
		"SinceRecent":   time.Since(recentMessage.CreatedAt),
		"RecentFrom":    recentMessage.From,
		"RepoInfo":      repoinfo,
	}
}

func runUname() (string, error) {
	out, err := exec.Command("uname", "-a").Output()
	return string(out), err
}

func runBashInfo() (string, error) {
	const projectget = `echo "Current Project Info"; date; set -x; tree -L2; find ./pytools ./pkg ./cmd -name '*.py' -or -name '*.go' ; bat --style=+header --style=+header-filename --decorations=always --paging=never --wrap=never --terminal-width=60  README.md; git log -n 3; echo end of 'git log -n 3'; echo; echo end of project info`
	// const projectget = `echo "Current Project Info"; date; set -x; tree -L2; find ./pytools ./pkg ./cmd -name '*.py' -or -name '*.go' ; bat --style=+header --style=+header-filename --decorations=always --paging=never --wrap=never --terminal-width=60  README.md; git log -n 3; scc`
	out, err := exec.Command("bash", "-c", projectget).Output()
	return string(out), err
}

func runFastfetch() (string, error) {
	// fastfetch --logo none
	out, err := exec.Command("fastfetch", "--logo", "none").Output()
	return string(out), err
}

var (
	cleanenv []string
	uname    string
)
