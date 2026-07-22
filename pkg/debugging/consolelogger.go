package debugging

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"m/pkg/llm"

	"github.com/lmittmann/tint"
)

type ConsoleLogger struct {
	HideThinking  bool
	HideToolCalls bool
	emode         modeT
	buf           string
	pre           string
	w             io.Writer
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
	ColorGreen = "\033[32m"
	ColorCyan  = "\033[34m"
	ColorReset = "\033[0m"
)

var (
	Colorcomment = ColorGray
	Colorcontent = ColorGreen
)

func NewConsoleLogger(ww ...io.Writer) *ConsoleLogger {
	w := Writer
	if len(ww) == 1 {
		w = ww[0]
	}
	if w == nil {
		w = io.Discard
	}
	return &ConsoleLogger{w: w, pre: ""}
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
	c.PrintChat(c.w, cr)
	return nil
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

func (c *ConsoleLogger) PrintMessage(w io.Writer, m llm.Message) {
	defer func() {
		if flusher, ok := w.(interface{ Flush() error }); ok {
			if err := flusher.Flush(); err != nil {
				panic(err.Error())
			}
		}
	}()
	/*
		if m.Thinking == "" && m.Content == "" && len(m.ToolCalls) == 0 {
			if len(c.buf) != 0 {
				got, ok := translation.MaybeRepairOutputRegex(c.buf)
				if ok && len(got) != 0 {
					_, _ = fmt.Fprintf(w, "%s", c.buf)
					c.buf = ""
					c.emode = 0
				}
			}
			return
		}
	*/

	// no content, yes thinking
	if m.Content == "" && m.Thinking != "" && c.emode == 0 {
		c.buf += m.Thinking
		/*
			if got, ok := translation.MaybeRepairOutputRegex(c.buf); ok && len(got) != 0 {
				c.buf = ""
				m.Content += got
			}
		*/
		if len(c.buf) > 32 { // got it
			m.Thinking = c.buf
			c.emode = xGO
			if !c.HideThinking {
				_, _ = fmt.Fprintf(w, "%s%s", Colorcomment, c.pre)
			}
		}
	}
	if c.emode != 0 && len(m.Thinking) != 0 && !c.HideThinking {
		_, _ = fmt.Fprintf(w, "%s", m.Thinking)
	}
	if m.Content != "" {
		if c.emode != xCONTENT {
			c.emode = xCONTENT
			if !c.HideThinking {
				_, _ = fmt.Fprintf(w, "\n")
			}
			_, _ = fmt.Fprintf(w, "%s%s", Colorcontent, c.pre)
		}
		_, _ = fmt.Fprintf(w, "%s", m.Content)
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
			_, _ = fmt.Fprintf(w, "\n%s```json\n%s(args=%s)\n```\n", c.pre, wraps[0].Function.Name, string(wraps[0].Function.Arguments))
		}
	}
}

func (c *ConsoleLogger) Reset() {
	c.emode = 0
	c.buf = ""
}

var noSummaries = os.Getenv("LLM_SUMMARY") != "1"

func (c *ConsoleLogger) PrintChat(w io.Writer, cr llm.ChatResponse) {
	if cr.Done {
		c.emode = xDONE
	}
	//if cr.DebugInfo != nil {
	//	fmt.Fprintf(os.Stderr, "%s", cr.DebugInfo.RenderedTemplate)
	//}
	if !c.HideToolCalls || len(cr.Message.ToolCalls) == 0 {
		c.PrintMessage(w, cr.Message)
	}
	if cr.Done {
		_, _ = fmt.Fprintf(w, "%s", ColorReset)
		//		if !c.HideToolCalls || len(cr.Message.ToolCalls) == 0 {
		//			_, _ = fmt.Fprintf(w, "\n")
		//		}

		//if !noSummaries && !llm.DisableLog {
		//	cr.Summary()
		//}
		c.Reset()
	}
}

var loglevel *slog.LevelVar

var Writer io.Writer = os.Stderr

// Init color slog
//
// Init().Set(slog.LevelInfo) or use ptr for flag etc
func Init(loglevel1 ...*slog.LevelVar) *slog.LevelVar {
	//if loglevel != nil {
	//	return loglevel
	//}
	if len(loglevel1) == 1 {
		loglevel = loglevel1[0]
	} else {
		loglevel = &slog.LevelVar{}
		loglevel.Set(slog.LevelDebug)
	}
	if Writer == nil {
		Writer = os.Stderr
	}
	//slog.SetLogLoggerLevel(slog.LevelDebug) // for now to use less
	x := tint.NewHandler(Writer, &tint.Options{AddSource: true, Level: loglevel, TimeFormat: "ORCHARD"})
	slog.SetDefault(
		slog.New(x),
	)
	return loglevel
}
