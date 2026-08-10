package debugging

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"m/pkg/global"
	"m/pkg/llm"

	"github.com/aerth/mostly/stackerr"
)

type ConsoleLogger struct {
	HideThinking  bool
	HideToolCalls bool
	emode         modeT
	buf           string
	pre           string
	w, tw         io.Writer
	NoColor       bool
}
type modeT int

const (
	xWAIT modeT = iota // print all but wait 32 bytes for thoughts
	xGO                // print all
	xCONTENT
	xDONE
)

const (
	ColorGray  = "\033[90m"
	ColorGreen = "\033[33m"
	ColorCyan  = "\033[34m"
	ColorReset = "\033[0m"
)

func NewConsoleLogger(ww ...io.Writer) *ConsoleLogger {
	hidetoolcall := global.Get().Get().HideToolCalls
	nocolor := global.Get().Get().NoColor || os.Getenv("NO_COLOR") != ""
	w := io.Writer(os.Stderr)
	tw := w
	if len(ww) == 1 {
		w = ww[0]
		tw = ww[0]
	}
	if len(ww) == 2 {
		w = ww[0]
		tw = ww[1]
	}

	if w == nil {
		w = io.Discard
	}
	if hidetoolcall {
		slog.Warn("hiding tool calls")
	}
	return &ConsoleLogger{w: w, tw: tw, pre: "", NoColor: nocolor, HideToolCalls: hidetoolcall}
}

func (c ConsoleLogger) WithPrefix(prefix string) ConsoleLogger {
	c.pre = prefix
	return c
}

func (c ConsoleLogger) Writer() io.Writer {
	return c.w
}

// commentary<|channel|>final<|message|>OK!
//

func (c *ConsoleLogger) HandleChatResponse(cr llm.ChatResponse) error {
	return c.PrintChat(c.w, c.tw, cr)
}

/*
func (c *ConsoleLogger) HandleGenerateResponse(cr llm.GenerateResponse) {
	c.PrintGenerate(c.w, cr)
}

func (c *ConsoleLogger) PrintGenerate(w io.Writer, cr llm.GenerateResponse) {
	c.PrintMessage(w, llm.Message{Thinking: cr.Thinking, Content: cr.Response})
}*/

func (c *ConsoleLogger) Mode() int {
	return int(c.emode)
}

func fflush(w io.Writer) {
	if flusher, ok := w.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			panic(err.Error())
		}
	}
}

func (c *ConsoleLogger) PrintMessage(w, tw io.Writer, m llm.Message) error {
	defer func() {
		// flush required for some writers
		fflush(w)
		fflush(tw)
	}()
	// no content, yes thinking
	if m.Content == "" && m.Thinking != "" && c.emode == 0 {
		c.buf += m.Thinking
		if len(c.buf) > 32 { // got it
			m.Thinking = c.buf
			c.emode = xGO
			if !c.HideThinking && !c.NoColor {
				_, _ = fmt.Fprintf(tw, "%s", ColorGray) // start colorize
			}
		}
	}
	// thinking actual
	if c.emode != 0 && len(m.Thinking) != 0 && !c.HideThinking {
		_, _ = fmt.Fprintf(tw, "%s", m.Thinking)
	}
	if m.Content != "" {
		if c.emode != xCONTENT { // first content chunk of this stream since last Reset
			if !c.HideThinking && c.emode == xGO {
				slog.Info("newline")
				_, _ = fmt.Fprintf(tw, "\n")
			}
			if !c.NoColor {
				_, err := fmt.Fprintf(w, "%s", ColorGreen)
				if err != nil {
					return stackerr.Errorf("%w", err)
				}
			}
			c.emode = xCONTENT
		}
		// already in content mode
		_, err := fmt.Fprintf(w, "%s", m.Content)
		if err != nil {
			return stackerr.Errorf("%w", err)
		}
	}
	if !c.HideToolCalls && len(m.ToolCalls) != 0 { // ollama api returns a full, parsed tool call(s). TODO: future: reimplement chat template for streaming the building of the tool call
		type namewrap struct {
			Function struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
		}
		wraps := make([]namewrap, len(m.ToolCalls))
		var err error
		for i := range m.ToolCalls {
			if err := json.Unmarshal(m.ToolCalls[i], &wraps[i]); err != nil {
				slog.Error("oops", "err", err)
			}

			_, err = fmt.Fprintf(w, "%s\ntoolcall\n```json\n%s(args=%s)\n```\n", c.pre, wraps[0].Function.Name, string(wraps[0].Function.Arguments))
			if err != nil {
				return stackerr.Errorf("%w", err)
			}
		}
	}
	return nil
}

func (c *ConsoleLogger) Reset() {
	c.emode = 0
	c.buf = ""
}

func (c *ConsoleLogger) PrintChat(w, tw io.Writer, cr llm.ChatResponse) error {
	if cr.Done {
		c.emode = xDONE
	}
	//if cr.DebugInfo != nil {
	//	fmt.Fprintf(os.Stderr, "%s", cr.DebugInfo.RenderedTemplate)
	//}
	//if !c.HideToolCalls || len(cr.Message.ToolCalls) == 0 {
	//
	if !cr.Message.IsEmpty() {
		//	slog.Info("printing", "text", cr.Message.Content, "is_done", cr.Done)
		if err := c.PrintMessage(w, tw, cr.Message); err != nil {
			slog.Error("printing message failed", "err", err)
			if cr.Done {
				c.Reset()
			}
			return stackerr.Errorf("%w", err)
		}
	} else {
		slog.Info("notprinting", "text", cr.Message.Content, "is_done", cr.Done)
	}
	//}
	var err error
	if cr.Done {
		defer c.Reset() // auto reset thinking colors after Done
		_, err = fmt.Fprintf(w, "%s", ColorReset)
		if err != nil {
			return stackerr.Errorf("%w", err)
		}
		if tw != w {
			_, err = fmt.Fprintf(tw, "%s", ColorReset)
			if err != nil {
				return stackerr.Errorf("%w", err)
			}
		}

		//		if !c.HideToolCalls || len(cr.Message.ToolCalls) == 0 {
		//			_, _ = fmt.Fprintf(w, "\n")
		//		}

		//if !noSummaries && !llm.DisableLog {
		//	cr.Summary()
		//}
	}
	return nil
}
