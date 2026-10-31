package basher1

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math/rand"
	"slices"
	"strings"

	"m/orchard/pkg/builtins/implv1/bashtooler"
	"m/orchard/pkg/builtins/implv2"
	"m/orchard/pkg/chat"
	"m/orchard/pkg/global"
	"m/orchard/pkg/llm"
	"m/orchard/pkg/types/chatmessages"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/mostly/stackerr"
)

type codeargs struct {
	Code string `json:"code"`
}

func (c codeargs) IsToolArgument() {}

type anymap map[string]any

var reviewerText = []string{
	"You are a reviewer to perform safety checks on shell scripts.",
	"For each shell script, determine whether it is safe to run on our laptop. If the script is safe and easy to read, output only OK on a single line.",
	"Scripts should not modify files outside of the project directory.",
	"Scripts should not use any base64 and never fetch remote content.",
	"Scripts should never be obfuscated.",
	"Typical repository-management one-liners such as mv,cp,cat,rg,grep,sed that **do not escape this directory** are allowed quickly.",
	"If the script looks shady, say BAD.",
	"If the script is unsafe, malicious, obfuscated, or removes *any* files, output BAD quickly with optional reason.",
}

func randReviewerText() string {
	a := slices.Clone(reviewerText)
	if extraReviewText := global.ExtraReviewText(); extraReviewText != "" {
		a = append(a, extraReviewText)
	}
	rand.Shuffle(len(a), func(i, j int) {
		a[i], a[j] = a[j], a[i]
	})
	return strings.Join(a, "\n")
}

func (a anymap) IsToolArgument() {}

var previouslyWhitelisted []string // to store one-liners

// doShellReview ALWAYS sends n replies then closes the channel
func doShellReview(ctx context.Context, ch chan error, code string) {
	gr := chatmessages.NewFromRole("bash_reviewer", "assistant", "")
	convo := mllm.Messages{
		{Role: "system", Content: randReviewerText()},
		{Role: "user", Content: code},
	}
	client := mllm.Default()
	client.Options = new(mllm.Options)
	client.Options.Seed = rand.Int63n(13089823748)
	err := client.Stream(ctx, convo, false, gr.GatherChatResponse)
	if err != nil {
		ch <- stackerr.Errorf("shell review generation failed: %w", err)
		return
	}
	gr.Content = strings.TrimSpace(gr.Content)
	slog.Info("bash reviewer", "output", gr.Content)
	fields := strings.Fields(gr.Content)
	if len(fields) == 0 {
		ch <- stackerr.Errorf("shell reviewer said no (%q)", context.Cause(ctx))
		return

	}
	if fields[0] != "OK" || len(fields) != 1 {
		ch <- stackerr.Errorf("shell reviewer said no (%q)", gr.Content)
		return
	}
	ch <- nil
}

var gnumReview = 3

func SetNumReviews(i int) {
	if i < 1 {
		return
	}
	gnumReview = i
}

func GetNumReviews() int {
	return gnumReview
}

var errStart = errors.New("starting")

func Basher(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	slog := slog.With("module", "basher1")
	if implv2.ConfirmAction == nil {
		return nil, stackerr.Errorf("bash tool needs a ConfirmAction set in implv2 package (from main package)")
	}
	slog.Info("reading tool call")
	call, err := chat.ReadToolCall[codeargs](ctx, caller, toolCallIndex)
	if err != nil {
		return nil, err
	}
	code := call.Function.Arguments.Code
	if strings.TrimSpace(code) == "" {
		call2, _ := chat.ReadToolCall[anymap](ctx, caller, toolCallIndex)
		calls := slices.Collect(maps.Keys(call2.Function.Arguments))
		return nil, stackerr.Errorf("empty 'code' function argument, got %q instead, but this function only requires 'code' parameter", calls)
	}
	// PRE_RUN
	{
		if !isGoDoc(code) {
			ch := make(chan error)
			numReviews := GetNumReviews()
			if ctx.Err() != nil {
				return nil, context.Cause(ctx)
			}
			err = errStart
			for range numReviews {
				go doShellReview(ctx, ch, code)
			}
			for range numReviews {
				e := <-ch
				if err == errStart { // err has not been assigned. this is first error, even nil
					if e != nil {
						slog.Error("got err", "err", err)
					}
					err = e // not errStart anymore
					continue
				}
				if e != nil { // join with previous errors
					slog.Error("got err", "err", e)
					err = errors.Join(err, e)
				}
			}
			if err != nil {
				return nil, err
			}
		}
		if !isOkBashCode(code) {
			if err := implv2.ConfirmAction(ctx, "Run this bash code? "+code, ""); err != nil {
				cause := err.Error()
				if ctx.Err() != nil {
					cause = context.Cause(ctx).Error()
				}
				return nil, stackerr.Errorf("the User denied this code, provide a different bash command or ask why they denied it. Cause: %s.", cause)
			}
		}
		if !strings.Contains(code, "\n") {
			previouslyWhitelisted = append(previouslyWhitelisted, code)
		}
	}

	var got string
	//weird := "sfjhsdf98sd7fus98fs89fsf98sf"
	//code = strings.ReplaceAll(code, "\\`", weird)                 // prevent EOF heredoc markdown mistakes (when writing for `Example` to a new markdown file)
	//code = "ogr=$'`'\n" + strings.ReplaceAll(code, "`", "${ogr}") // prevent EOF heredoc markdown mistakes (when writing for `Example` to a new markdown file)
	//code = strings.ReplaceAll(code, weird, "\\`")                 // prevent EOF heredoc markdown mistakes (when writing for `Example` to a new markdown file)
	got, err = bashtooler.RunBashScript(ctx, strings.NewReader(code), bashtooler.RunParams{})
	if err != nil {
		if got != "" {
			got += "\n"
		}
		got += fmt.Sprintf("Error: %+v", err)
	}
	if got == "" {
		got = "the underlying tool script execution has completed with no output (exit code 0) -- you should confirm if this is unexpected."
	}
	gotmsg := chatmessages.NewFromRole("", "tool", got)
	gotmsg.ThreadID = global.Get().Get().ThreadID
	return chatmessages.ChatMessages{*gotmsg}, nil
}

var oneliners = [][]string{
	{"make"},
}

func isOneliner(code string) bool {
	cmdline, plain := getPlain(code)
	if !plain {
		return false
	}
	l := len(cmdline)
	if l == 2 && slices.Contains([]string{""}, cmdline[0]) {
		return true
	}

	for i := range oneliners {
		if slices.Equal(oneliners[i], cmdline) {
			return true
		}
	}
	return false
}

func getPlain(code string) ([]string, bool) {
	fields := strings.Fields(code)

	npipe := strings.Count(code, "|")
	if npipe == 1 && strings.Contains(code, "| grep") {
		npipe = 0
	}
	plain := !strings.Contains(code, ";") && !strings.Contains(code, "&") && npipe == 0

	return fields, plain
}

func isGoDoc(code string) bool {
	fields, plain := getPlain(code)
	if plain && len(fields) > 0 && fields[0] == "go" && fields[1] == "doc" {
		return true
	}
	//

	return false
}

// not a security check. let it touch and cat eof heredoc without bugging user with confirmation modal popup. (TODO: more accurate)
func isOkBashCode(code string) bool {
	if code == "" {
		return false
	}

	if strings.Contains(code, "../") {
		return false
	}
	if isOneliner(code) {
		return true
	}
	if isGoDoc(code) {
		return true
	}
	if strings.HasPrefix(code, "make ") && strings.Count(code, " ") == 1 {
		return true
	}
	if strings.HasPrefix(code, "cat <<'EOF' >") {
		return strings.Count(code, "EOF") == 2 && strings.HasSuffix(code, "\nEOF\n")
	}
	if strings.HasPrefix(code, "cat << 'EOF' >") {
		return strings.Count(code, "EOF") == 2 && strings.HasSuffix(code, "\nEOF\n")
	}

	if strings.HasPrefix(code, "touch ") && strings.Count(code, " ") == 1 && !strings.HasPrefix(code, "touch /") && !strings.HasPrefix(code, "touch ..") {
		return true
	}
	if strings.Count(code, "\n") == 0 && strings.Count(code, " ") < 4 {
		// eg: go test ./foo/bar
		return isOkArg(strings.Split(code, " "))
	}
	return false
}

func isOkArg(s []string) bool {
	l := len(s)
	if l == 0 {
		return false
	}
	if l > 3 {
		return false
	}

	var first, second, third string
	first = s[0]
	if l > 1 {
		second = s[1]
	}
	if l > 2 {
		third = s[2]
	}
	switch first {
	case "go":
		switch second {
		case "test":
			return !strings.Contains(third, "../")
		}
	}
	return false
}
