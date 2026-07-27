package implv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/llm"

	"github.com/aerth/mostly/stackerr"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

type haspath struct {
	Path string `json:"path"`
}

func (h haspath) IsToolArgument() {}
func EditTool(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	// begin loop

	call, err := chat.ReadToolCall[haspath](ctx, caller, toolCallIndex)
	if err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	path := call.Function.Arguments.Path

	errmsg := llm.Message{
		Role:     "tool",
		Content:  "no output",
		ToolName: "edit_file", ToolCallID: caller.ToolCallID,
	}
	if path == "" {
		errmsg.Content = "Error: path required"
		return chatmessages.A{*chatmessages.New(errmsg)}, nil
	}

	// begin loop
	var out chatmessages.A
	session := Session{ActiveFile: path}
	if e := session.Refresh().E; e != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	stack := chatmessages.A{
		*chatmessages.New(llm.Message{Role: "system", Content: SystemInstruction()}),
	}
	//e := chat.Ext{
	//	NonStreamHandler: func(cm chatmessages.ChatMessage) {
	//	}
	//}
	//
	var operators []string
	creq := llm.NewChatRequest()
	var card string
	var final *chatmessages.ChatMessage
	for ctx.Err() == nil {
		creq.Messages = stack.RemoveEphemeral().Raw()
		if card != "" {
			creq.Messages = append(creq.Messages, llm.Message{Role: "system", Content: card})
			card = ""
			for i := range creq.Messages[1 : len(creq.Messages)-1] {
				if creq.Messages[i].Role == "system" && strings.Count(creq.Messages[i].Content, "\n") > 1 {
					creq.Messages[i].Content = strings.Split(creq.Messages[i].Content, "\n")[0]
				}
			}
		}
		newmsg := chatmessages.NewFromRole("", "assistant", "")
		err := llm.New().Chat(ctx, creq)
		if err != nil {
			operators = append(operators, err.Error())
			break
		}
		if newmsg.Content == "" {
			operators = append(operators, "no output")
			break
		}
		stack = append(stack, *newmsg)
		x, err := session.Parse(newmsg.Content)
		if errors.Is(err, ErrNot) && session.LockEnd == 0 {
			// finished. latest was a "I have corrected the typos in `foo.txt` while maintaining the exact number of lines." message
			final = newmsg
			break
		}
		if errors.Is(err, ErrNot) && session.LockEnd != 0 {
			// target acquired
			// TODO: count lines vs locked lines?
			err = session.ApplySurgicalEdit(strings.Split(newmsg.Content, "\n"))
			if err == nil && !strings.HasSuffix(session.ActiveFile, ".txt") {
				lang, langname, _ := getlang(session.ActiveFile)
				if lang != nil {
					err = ValidateFileSyntax(string(session.Current()), langname, lang)
				}
			}
			if err == nil {
				err = os.WriteFile(session.ActiveFile, session.Current(), 0o600)
			}
			if err == nil {
				session.LockStart = 0
				session.LockEnd = 0
				creq.Messages = append(creq.Messages, llm.Message{
					Role:    "system",
					Content: "The file edits were applied to the filesystem",
				})
				break
			}

		}
		if err != nil {
			slog.Error("parsing edit TODO", "err", err)
			operators = append(operators, fmt.Sprintf("ERROR: %+v", err))
			break
		}
		for i := range x {
			operators = append(operators, x[i].Op)
			resp := session.RunAction(x[i])
			if resp.E != nil {
				operators = append(operators, fmt.Sprintf("ERROR: %+v", resp.E))
				break
			}
			card = resp.Message
		}
	}

	x := jsonstring(operators)
	errmsg.Content = fmt.Sprintf("The File edit session (%q) has completed. Transcript: %s", path, x)
	out = append(out, *chatmessages.New(errmsg))
	if final != nil {
		out = append(out, *final)
	} else {
		out = append(out, stack[len(stack)-1])
	}
	slog.Info("returning messages", "n", len(out))
	return out, nil
}

func getpath(rawMessage json.RawMessage) string {
	var x haspath
	_ = json.Unmarshal(rawMessage, &x)
	return x.Path
}

func jsonstring(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func foo(stack chatmessages.A) chatmessages.A {
	beginLength := len(stack)
	x := chatmessages.New(llm.Message{Role: "system", Content: SystemInstruction()})
	x.Ephemeral = true

	return stack[beginLength:]
}

func getlang(s string) (*sitter.Language, string, error) {
	if !strings.Contains(s, ".") {
		return nil, "", fmt.Errorf("no dot")
	}
	ss := strings.Split(s, ".")
	ext := ss[len(ss)-1]
	switch ext {
	case "go":
		return golang.GetLanguage(), ext, nil
	case "py":
		return python.GetLanguage(), ext, nil
	case "html":
		return html.GetLanguage(), ext, nil
	case "ts":
		return typescript.GetLanguage(), ext, nil
	case "js":
		return javascript.GetLanguage(), ext, nil
	case "css":
		return css.GetLanguage(), ext, nil
	default:
		return nil, ext, fmt.Errorf("not implemented yet")
	}
}

/*
*
			if creq.Messages[len(creq.Messages)-1].Role == "assistant" {
				println("[test] test is done (latest message is assistant)")
				break
			}
			generated := chatmessages.NewFromRole("", "assistant", "")
			for i := range creq.Messages[1 : len(creq.Messages)-1] {
				if creq.Messages[i].Role == "system" && strings.Count(creq.Messages[i].Content, "\n") > 1 {
					creq.Messages[i].Content = strings.Split(creq.Messages[i].Content, "\n")[0]
				}
			}
			msgs := creq.Messages
			if testpass {
				_ = json.NewEncoder(os.Stderr).Encode(msgs)
			}
			err = llm.New().Chat(t.Context(), creq, generated.GatherChatResponse)
			creq.Messages = msgs
			if err != nil {
				t.Fatal(err)
			}
			println(generated.Content)
			creq.Messages = append(creq.Messages, generated.Message)
			actions, err := session.Parse(generated.Content)
			if errors.Is(err, implv2.ErrNot) && session.LockEnd == 0 {
				// finished. latest was a "I have corrected the typos in `foo.txt` while maintaining the exact number of lines." message
				break
			}
			if errors.Is(err, implv2.ErrNot) && session.LockEnd != 0 {
				// target acquired
				err = session.ApplySurgicalEdit(strings.Split(generated.Content, "\n"))
				if err == nil && !strings.HasSuffix(session.ActiveFile, ".txt") {
					lang, langname, _ := getlang(session.ActiveFile)
					if lang != nil {
						err = implv2.ValidateFileSyntax(string(session.Current()), langname, lang)
					}
				}
				if err == nil {
					if session.ActiveFile != tc.Filename {
						t.Fatal("only one file can be edited but got " + session.ActiveFile)
					}
					err = os.WriteFile(session.ActiveFile, session.Current(), 0o600)
					testpass = err == nil
				}
				session.LockStart = 0
				session.LockEnd = 0
				creq.Messages = append(creq.Messages, llm.Message{
					Role:    "system",
					Content: "The file edits were applied to the filesystem",
				})

			}
			if err != nil {
				t.Fatalf("%+v", err)
			}
			for i := range actions {
				if actions[i].Op == implv2.OpCancel {
					slog.Info("cancel requested")
					return
				}
				slog.Info("got action", "i", i, "s", actions[i].Op, "x", strings.Split(actions[i].Content, "\n")[0])
				res := session.RunAction(actions[i])
				if res.E != nil {
					t.Fatalf("error: %+v", err)
				}
				slog.Info("result", "out", res.Code, "message", res.Message)
				if res.Message == "" {
					res.Message = SuccessMessage
				}
				creq.Messages = append(creq.Messages, llm.Message{
					Role:    "system",
					Content: res.Message + "\n" + session.SystemMessage(),
				})
				println(creq.Messages[len(creq.Messages)-1].Content)
			}
*/
