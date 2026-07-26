package debugging

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"m/pkg/global"
	"m/pkg/llm"
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
	return &ConsoleLogger{w: w, tw: tw, pre: "", NoColor: nocolor}
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
		if c.emode != xCONTENT {
			if !c.HideThinking && c.emode == xGO {
				_, _ = fmt.Fprintf(tw, "\n")
			}
			if !c.NoColor {
				_, err := fmt.Fprintf(w, "%s", ColorGreen)
				if err != nil {
					return err
				}
			}
			c.emode = xCONTENT
		}
		_, err := fmt.Fprintf(w, "%s", m.Content)
		if err != nil {
			return err
		}
	}
	if len(m.ToolCalls) != 0 { // ollama api returns a full, parsed tool call(s). TODO: future: reimplement chat template for streaming the building of the tool call
		type namewrap struct {
			Function struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
		}
		wraps := make([]namewrap, len(m.ToolCalls))
		for i := range m.ToolCalls {
			if err := json.Unmarshal(m.ToolCalls[i], &wraps[i]); err != nil {
				slog.Error("oops", "err", err)
			}

			_, err = fmt.Fprintf(w, "\n%s```json\n%s(args=%s)\n```\n", c.pre, wraps[0].Function.Name, string(wraps[0].Function.Arguments))
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
	if !c.HideToolCalls || len(cr.Message.ToolCalls) == 0 {
		if err := c.PrintMessage(w, tw, cr.Message); err != nil {
			if cr.Done {
				c.Reset()
			}
			return err
		}
	}
	var err error
	if cr.Done {
		defer c.Reset()
		_, err = fmt.Fprintf(w, "%s", ColorReset)
		if err != nil {
			return err
		}
		if tw != w {
			_, err = fmt.Fprintf(tw, "%s", ColorReset)
			if err != nil {
				return err
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
