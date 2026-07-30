package implv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"m/pkg/chat"
	"m/pkg/chatmessages"
	"m/pkg/debugging"
	"m/pkg/global"
	"m/pkg/llm"
	"m/pkg/otool"
	"m/pkg/toolrunner"
	"os"
	"strings"

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
	Path   string `json:"path,omitempty"`
	Intent string `json:"intent,omitempty"`
}

var AskBeforeWriting = false
var (
	GetHandler = func() chat.ChatHandlerPlus { return nil }
	GetSender  = func(Result) {}
	PreFunc    = func() {}
)

func (h haspath) IsToolArgument() {}

type EditorTool struct {
	session *Session
	Ext     chat.Ext
}

func NewEditTool() *EditorTool {
	edit := &EditorTool{
		session: &Session{},
	}
	edit.session.editor = edit
	return edit
}

func (s *Session) EditTool() otool.ExecFunc {
	return s.editor.EditTool
}

func EditTool(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	return nil, fmt.Errorf("wrong tool")
}

func (e *EditorTool) EditTool(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	// begin loop

	call, err := chat.ReadToolCall[haspath](ctx, caller, toolCallIndex)
	if err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	path := call.Function.Arguments.Path
	intent := call.Function.Arguments.Intent

	errmsg := llm.Message{
		Role:     "system",
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
	slog := slog.With("filename", path)
	if e := session.Refresh().E; e != nil {
		slog.Error("refresher error", "err", e)
		return nil, stackerr.Errorf("%w", e)
	}
	stack, _ := ctx.Value(toolrunner.CtxStack).(chatmessages.ChatMessages)
	if l := len(stack); l > 4 {
		stack = stack[l-4:]
	}
	startstacklen := len(stack)
	if startstacklen == 0 {
		return nil, fmt.Errorf("no conversation memory")
	}
	stack = append(stack, chatmessages.A{
		*chatmessages.NewSystemEphemeral(SystemInstruction()), // global no state
	}...)
	//e := chat.Ext{
	//	NonStreamHandler: func(cm chatmessages.ChatMessage) {
	//	}
	//}
	//
	var operators []string
	creq := llm.NewChatRequest()
	var card string
	cl := debugging.NewConsoleLogger(global.LogWriter, global.LogWriter)
	rounds := 0
	numberFilesEdited := 0
	handler := GetHandler()
	for ctx.Err() == nil {
		slog := slog.With("lock", fmt.Sprintf("%d-%d", session.LockStart, session.LockEnd))
		creq.Messages = stack.Raw()
		tmpmsg := strings.Join([]string{card, session.SystemMessage()}, "\n")
		card = ""
		log.Printf("%s", tmpmsg)
		creq.Messages = append(creq.Messages, llm.Message{Role: "system", Content: tmpmsg})
		for i := range creq.Messages[1+startstacklen : len(creq.Messages)] {
			if creq.Messages[i].Role == "system" && strings.Count(creq.Messages[i].Content, "\n") > 1 {
				creq.Messages[i].Content = strings.Split(creq.Messages[i].Content, "\n")[0]
			}
		}
		newmsg := chatmessages.NewFromRole("editor_agent", "assistant", "")
		handlers := []chat.Handler{cl.HandleChatResponse, newmsg.GatherChatResponse}
		if handler != nil {
			handlers = append(handlers, handler.Handle)
		}
		if e.Ext.PreFunc != nil {
			if err := e.Ext.PreFunc(creq); err != nil {
				return nil, err
			}
		}
		err := llm.New().Chat(ctx, creq, handlers...)
		cl.Reset()
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
		if rounds != 0 && errors.Is(err, ErrNot) && session.LockEnd == 0 {
			// finished. latest was a "I have corrected the typos in `foo.txt` while maintaining the exact number of lines." message
			slog.Info("Finished editing")
			break
		}
		if errors.Is(err, ErrNot) && session.LockEnd != 0 {
			// target acquired
			// TODO: count lines vs locked lines?
			err = session.ApplySurgicalEdit(strings.Split(newmsg.Content, "\n"))
			if err != nil {
				errmsg.Content = fmt.Sprintf("ApplySurgicalEdit failed: %+v", err)
				continue
			}
			// in memory still. check lint
			if !strings.HasSuffix(session.ActiveFile, ".txt") {
				lang, langname, _ := getlang(session.ActiveFile)
				if lang != nil {
					err = ValidateFileSyntax(string(session.Current()), langname, lang)
					if err != nil {
						errmsg.Content = fmt.Sprintf("ValidateFileSyntax failed: %+v", err)
						continue
					}
				}
			}
			if ConfirmAction != nil && AskBeforeWriting {
				err = ConfirmAction(ctx, "EDDITOR EDITING Lines: "+newmsg.Content)
				if err != nil {
					errmsg.Content = fmt.Sprintf("ConfirmAction failed: %+v", err)
					continue
				}
			} else {
				slog.Warn("yolo edit mode?", "filename", session.ActiveFile)
			}
			slog.Info("WRITING FILE")
			err = os.WriteFile(session.ActiveFile, session.Current(), 0o600)
			if err == nil {
				session.LockStart = 0
				session.LockEnd = 0
				stack = append(stack, *chatmessages.New(llm.Message{
					Role:    "system",
					Content: "The file edits were applied to the filesystem",
				}))
				break
			}
			numberFilesEdited++

		}
		if err != nil {
			slog.Error("parsing edit TODO", "err", err)
			stack = append(stack, *chatmessages.NewSystemEphemeral("Error: " + err.Error() + "\n\nTry again or use CANCEL to return to conversation"))
			continue
		}
		responseWriter := GetSender
		for i := range x {
			slog.Info("running action", "op", x[i].Op)
			operators = append(operators, x[i].Op)
			resp := session.RunAction(x[i])
			if responseWriter == nil && handler != nil {
				handler.HandleToolOutputs(NewResultMessage(resp))
			}
			if responseWriter != nil {
				responseWriter(resp)
			}
			if resp.E != nil {
				operators = append(operators, fmt.Sprintf("ERROR: %+v", resp.E))
				break
			}

		}
	}

	/*
		x := jsonstring(operators)
		x += fmt.Sprintf("files edited: %d", numberFilesEdited)
		if final == nil {
			y := stack.Top()
			final = &y
		}
		errmsg.Content = fmt.Sprintf("The editing session (%q) has completed.Transcript: %s\n\nFinal:\n%s", path, x,
			final.FormatMarkdownTry(nil, "", "Editor"))*/
	errmsg.Content = fmt.Sprintf("## Transcript of editor session (abbreviated, not raw)\n\n```\n%s\n```\n\n### Operation keywords found: %q\n\nMost recent file view:\n\n```\n%s```\n", stack[startstacklen:].FormatText("", ""), operators, session.CurrentView())
	out = append(out, *chatmessages.New(errmsg))
	slog.Info("editor returning messages", "n", len(out))
	return out, nil
}

func NewResultMessage(resp Result) chatmessages.ChatMessage {
	msg := *chatmessages.NewAux(resp)
	return msg
}

var ConfirmAction = func(ctx context.Context, s string) error {
	return fmt.Errorf("there is no ConfirmAction function provided")
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
