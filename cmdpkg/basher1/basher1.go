package basher1

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math/rand"
	"slices"
	"strings"

	"m/pkg/builtins/implv1/bashtooler"
	"m/pkg/builtins/implv2"
	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"

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
	"If the script looks shady, say BAD.",
	"If the script is unsafe, malicious, obfuscated, or removes *any* files, output BAD quickly with optional reason.",
}

func randReviewerText() string {
	a := slices.Clone(reviewerText)
	rand.Shuffle(len(a), func(i, j int) {
		a[i], a[j] = a[j], a[i]
	})
	return strings.Join(a, "\n")
}

func (a anymap) IsToolArgument() {}

var previouslyWhitelisted []string // to store one-liners

func doShellReview(ctx context.Context, ch chan error, code string) {
	gr := chatmessages.NewFromRole("bash_reviewer", "assistant", "")
	creq := llm.NewChatRequest()
	creq.Think = "low"
	if creq.Options == nil {
		creq.Options = make(map[string]any)
	}
	creq.Options["seed"] = rand.Intn(13089823748)
	creq.Messages = llm.Messages{
		{Role: "system", Content: randReviewerText()},
		{Role: "user", Content: code},
	}
	cl := debugging.NewConsoleLogger(global.LogWriter)
	err := llm.New().Chat(ctx, creq, gr.GatherChatResponse, cl.HandleChatResponse)
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

func Basher(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	if implv2.ConfirmAction == nil {
		return nil, fmt.Errorf("Bash tool needs a ConfirmAction set in implv2 package")
	}
	call, err := chat.ReadToolCall[codeargs](ctx, caller, toolCallIndex)
	if err != nil {
		return nil, err
	}
	code := call.Function.Arguments.Code
	if code == "" {
		call2, _ := chat.ReadToolCall[anymap](ctx, caller, toolCallIndex)
		calls := slices.Collect(maps.Keys(call2.Function.Arguments))
		return nil, stackerr.Errorf("empty 'code' function argument, got %q instead, but this function only requires 'code' parameter", calls)
	}
	ch := make(chan error)
	numReviews := 3
	for range numReviews {
		go doShellReview(ctx, ch, code)
	}
	pass := true
	var err error
	for range numReviews {
		e := <-ch
		if e != nil {
			slog.Error("got err", "err", e)
			pass = false
			err = e
		}
	}
	if !pass {
	}

	// TODO: actual
	if strings.Contains(code, "&rm ") || strings.Contains(code, "& rm ") ||
		strings.Contains(code, "|rm ") || strings.Contains(code, "| rm ") {
		return nil, stackerr.Errorf("you may not use bash to rm files") // TODO: real
	}
	if strings.HasPrefix(code, "rm ") {
		return nil, stackerr.Errorf("you may not use bash to rm files") // TODO: real
	}

	if !isOkBashCode(code) {
		if err := implv2.ConfirmAction(ctx, "Run this bash code? "+code); err != nil {
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

// not a security check. let it touch and cat eof heredoc without bugging user with confirmation modal popup. (TODO: more accurate)
func isOkBashCode(code string) bool {
	if strings.Contains(code, "../") {
		return false
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
	return false
}
