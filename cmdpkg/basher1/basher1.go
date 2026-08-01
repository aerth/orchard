package basher1

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"m/cmdpkg"
	"m/pkg/builtins/implv1/bashtooler"
	"m/pkg/builtins/implv2"
	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/global"
	"m/pkg/llm"

	"github.com/aerth/mostly/stackerr"
)

type codeargs struct {
	Code string `json:"code"`
}

func (c codeargs) IsToolArgument() {}

type anymap map[string]any

func (a anymap) IsToolArgument() {}
func Basher(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
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
	// TODO: actual
	if strings.Contains(code, "&rm ") || strings.Contains(code, "& rm ") ||
		strings.Contains(code, "|rm ") || strings.Contains(code, "| rm ") {
		return nil, fmt.Errorf("you may not use bash to rm files") // TODO: real
	}
	if strings.HasPrefix(code, "rm ") {
		return nil, fmt.Errorf("you may not use bash to rm files") // TODO: real
	}
	if implv2.ConfirmAction == nil {
		implv2.ConfirmAction = cmdpkg.Cheatcode
	}

	if !isOkBashCode(code) {
		if implv2.ConfirmAction(ctx, "Run this bash code? "+code) != nil {
			return nil, stackerr.Errorf("the User denied this code, provide a different bash command or ask why they denied it. Cause: %s.", context.Cause(ctx))
		}
	}

	var got string
	weird := "sfjhsdf98sd7fus98fs89fsf98sf"
	code = strings.ReplaceAll(code, "\\`", weird)                 // prevent EOF heredoc markdown mistakes (when writing for `Example` to a new markdown file)
	code = "ogr=$'`'\n" + strings.ReplaceAll(code, "`", "${ogr}") // prevent EOF heredoc markdown mistakes (when writing for `Example` to a new markdown file)
	code = strings.ReplaceAll(code, weird, "\\`")                 // prevent EOF heredoc markdown mistakes (when writing for `Example` to a new markdown file)
	got, err = bashtooler.RunBashScript(ctx, strings.NewReader(code), bashtooler.RunParams{})
	if err != nil {
		if got != "" {
			got += "\n"
		}
		got += fmt.Sprintf("Error: %+v", err)
	}
	if got == "" {
		got = "command completed with no output (exit code 0) -- you should confirm that this command is meant to be silent"
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
	if strings.HasPrefix(code, "cat <<EOF >") {
		return strings.Count(code, "EOF") == 2 && strings.HasSuffix(code, "\nEOF\n")
	}
	if strings.HasPrefix(code, "cat <<EOF >>") {
		return strings.Count(code, "EOF") == 2 && strings.HasSuffix(code, "\nEOF\n")
	}
	if strings.HasPrefix(code, "touch ") && strings.Count(code, " ") == 1 && !strings.HasPrefix(code, "touch /") && !strings.HasPrefix(code, "touch ..") {
		return true
	}
	return false
}
