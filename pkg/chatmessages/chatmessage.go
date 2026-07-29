package chatmessages

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"m/pkg/llm"
	"m/pkg/vector256"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (cm ChatMessage) GetID() uint {
	return cm.DBID
}

type (
	RawCall     = Call[json.RawMessage]
	Call[T any] struct {
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments T      `json:"arguments,omitempty"`
		} `json:"function"`
	}
)

type ChatMessage struct {
	DBID           uint            `gorm:"primarykey;column:id" json:"dbid,omitempty"`                                     // The MESSAGE DATABASE ID INTERNAL UNUSED ORDERED
	ThreadID       uuid.UUID       `gorm:"type:uuid;not null;index:idx_thread_time,priority:1" json:"thread_id,omitempty"` // ThreadID for storing in DB
	MessageID      uuid.UUID       `gorm:"type:uuid;primaryKey;notnull;" json:"message_id,omitempty"`                      // MessageID is unique identifier for each message
	CreatedAt      time.Time       `json:"time"`
	UpdatedAt      time.Time       `json:"-"`
	DeletedAt      *gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	llm.Message    `gorm:"embedded" json:"message"`
	Embedding      *vector256.Vector256 `gorm:"type:vector(256);null" json:"-"`
	From           string               `json:"from,omitempty"` // name of assistant
	MessageOptions `gorm:"-" json:"-"`
}

// FormatText in dialogue text format with no newline ending
func (cm ChatMessage) FormatText(username string, botname string) string {
	from := cm.FromName(username, botname)
	return fmt.Sprintf("%s: %s", from, cm.Content)
}

func (cm ChatMessage) FromName(username string, botname string) string {
	from := cm.From
	if from != "" {
		return from
	}
	from = cm.Role
	if cm.Role == "user" && username != "" {
		from = username
	} else if cm.Role == "assistant" && botname != "" {
		from = botname
	}
	return from
}

func (cm *ChatMessage) ContentOrToolCall() string {
	if cm.Content != "" {
		return cm.Content
	}
	if len(cm.ToolCalls) == 1 {
		js, err := json.Marshal(cm.ToolCalls[0])
		if err != nil {
			return "(error)"
		}
		return "Tool Call: " + string(js)
	}
	if len(cm.ToolCalls) != 0 {
		js, err := json.Marshal(cm.ToolCalls)
		if err != nil {
			return "(error)"
		}
		return "Tool Calls: " + string(js)
	}
	return "(empty)"
}

func New(m llm.Message) *ChatMessage {
	return NewFrom("", m)
}

func NewChatMessage(role string, content string) *ChatMessage {
	return NewFrom("", llm.Message{Role: role, Content: content})
}

func NewFromRole(from, role, content string) *ChatMessage {
	return NewFrom(from, llm.Message{Role: role, Content: content})
}

func NewFrom(from string, m llm.Message) *ChatMessage {
	t := time.Now().UTC()
	return &ChatMessage{From: from, Message: m, CreatedAt: t, MessageID: uuid.New(), UpdatedAt: t}
}

func NewFromEphemeral(from string, m llm.Message) *ChatMessage {
	t := time.Now().UTC()
	return &ChatMessage{From: from, Message: m, CreatedAt: t, MessageID: uuid.New(), UpdatedAt: t, MessageOptions: MessageOptions{Ephemeral: true}}
}

func NewSystemEphemeral(content string) *ChatMessage {
	return NewFromEphemeral("", llm.Message{Role: "system", Content: content})
}

// NewToolReply with matching Call ID and ToolName for the given i index.
func NewToolReply(caller ChatMessage, i int, content string) *ChatMessage {
	var call Call[json.RawMessage]
	err := json.Unmarshal(caller.ToolCalls[i], &call)
	if err != nil {
		slog.Error("unmarshal tool call", "err", err)
		return nil
	}
	toolmsg := llm.Message{
		Role:       "tool",
		ToolCallID: call.ID,
		ToolName:   call.Function.Name,
		Content:    content,
	}
	return NewFrom(caller.From, toolmsg)
}

// MessageOptions are not saved to DB
type MessageOptions struct {
	Summary     any
	Metrics     llm.Metrics
	Aux         any
	Attachments []string // Attachments filenames
	Ephemeral   bool
	GenDuration time.Duration
}

func (cm ChatMessage) TableName() string {
	return "chat_messages_test"
}

func (cm *ChatMessage) GatherChatResponse(cr llm.ChatResponse) error {
	if cm == nil {
		panic("GatherChatResponse for nil")
	}
	if cr.Message.Role != "" {
		cm.Role = cr.Message.Role
	}
	cm.Thinking += cr.Message.Thinking
	cm.Content += cr.Message.Content
	if len(cr.Message.ToolCalls) == 0 && !cr.Done {
		return nil
	}
	if cm.ToolCallID == "" {
		cm.ToolCallID = cr.Message.ToolCallID
	}
	if len(cm.ToolCalls) != 0 && len(cr.Message.ToolCalls) != 0 {
		slog.Error("already", "prev", toJSON(cm.ToolCalls[0]), "next", toJSON(cr.Message.ToolCalls[0]))
		//	panic("wow already?")
	}
	if len(cr.Message.ToolCalls) != 0 {
		cm.ToolCalls = append(cm.ToolCalls, cr.Message.ToolCalls...)
	}
	if len(cm.ToolCalls) == 0 {
		cm.ToolCalls = nil
	}
	if cr.Done {
		slog.Info("Is Done", "reason", cr.DoneReason, "metrics", cr.Metrics)
		var buf bytes.Buffer
		cr.Summary(&buf)
		cm.Summary = buf.String()
		cm.Metrics = cr.Metrics
	}
	if cr.Message.Content == " (END)" {
		return fmt.Errorf("broken")
	}
	return nil
}

func toJSON(rawMessage json.RawMessage) string {
	b, err := json.Marshal(rawMessage)
	if err != nil {
		panic(err.Error())
	}
	return string(b)
}

func (cm ChatMessage) IsEmpty() bool {
	return cm.Thinking == "" && cm.Content == "" && len(cm.ToolCalls) == 0
}

func (cm ChatMessage) String() string {
	return "thr_" + cm.ThreadID.String()[:8] + "_msg_" + cm.MessageID.String()[:8] + "_" + cm.Message.String()
}

func (cm ChatMessages) Contents() string {
	var str strings.Builder
	for i := range cm {
		str.WriteString(cm[i].Content)
	}
	return str.String()
}

type ChatMessages []ChatMessage

func (cm ChatMessages) FormatRoles() []string {
	a := make([]string, len(cm))
	for i := range cm {
		a[i] = cm[i].Role
	}
	return a
}

func (cm ChatMessages) TopAttachment() ChatMessage {
	l := len(cm) - 1
	for i := range cm {
		if len(cm[l-i].Attachments) != 0 {
			return cm[l-i]
		}
	}
	return ChatMessage{}
}

func (cm ChatMessages) TopRole(role string) ChatMessage {
	l := len(cm) - 1
	for i := range cm {
		if cm[l-i].Role == role {
			return cm[l-i]
		}
	}
	return ChatMessage{}
}

func (cm ChatMessages) FormatText(username, botname string) string {
	var str strings.Builder
	for _, v := range cm {
		str.WriteString(v.FormatText(username, botname))
	}
	return str.String()
}

func (cm ChatMessages) TopAssistant() ChatMessage {
	return cm.TopRole("assistant")
}

func (cm ChatMessages) TopN(n int) ChatMessages {
	if n < 0 {
		panic("n must be 0>")
	}
	l := len(cm) - 1
	if l < n {
		return cm
	}
	return cm[l-n:]
}

func (cm ChatMessages) Top() ChatMessage {
	l := len(cm)
	if l == 0 {
		return ChatMessage{}
	}
	return cm[l-1]
}

func (cm ChatMessages) RemoveThoughts() ChatMessages {
	a := cm.Clone()
	for i := range a {
		a[i].Thinking = ""
	}
	return a
}

func (cm ChatMessages) RemoveEphemeral() ChatMessages {
	return slices.DeleteFunc(cm.Clone(), func(v ChatMessage) bool {
		return v.Ephemeral
	})
}

func (cm ChatMessages) RemoveSystemEphemeralThoughts() ChatMessages {
	a := slices.DeleteFunc(cm.Clone(), func(v ChatMessage) bool {
		return v.Ephemeral || v.Role == "system"
	})
	for i := range a {
		a[i].Thinking = ""
	}
	return a
}

func (cm ChatMessages) RemoveSystem() ChatMessages {
	return slices.DeleteFunc(cm.Clone(), func(v ChatMessage) bool {
		return v.Role == "system"
	})
}

func (cm ChatMessages) Clone() ChatMessages {
	return slices.Clone(cm)
}

type A = ChatMessages

func (cm ChatMessages) Raw() llm.Messages {
	a := make(llm.Messages, 0, len(cm))
	for i := range cm {
		if cm[i].Aux != nil || cm[i].IsEmpty() {
			slog.Warn("skipping raw()", "i", i)
			continue
		}

		//	slog.Warn("got raw", "i", i, "msg", cm[i].Message)
		a = append(a, cm[i].Message)
	}
	return a
}

func (cm ChatMessages) ContainsID(id uuid.UUID) bool {
	return slices.ContainsFunc(cm, func(v ChatMessage) bool {
		return v.MessageID == id
	})
}

type RendererFunc = func(in string, style int) (string, error)

var (
	ShowThoughts    = true
	ShowToolOutputs = true
)

func NewAux(aux any) *ChatMessage {
	x := New(llm.Message{})
	x.Aux = aux
	return x
}

func (cm ChatMessage) FormatMarkdownTry(renderfunc RendererFunc, username, botname string) string {
	got, err := cm.FormatMarkdown(renderfunc, username, botname)
	if err != nil {
		slog.Error("error formating markdown msg", "err", err)
		return got + " (ERROR: formatting markdown failed)"
	}
	return got
}

func (cm ChatMessage) FormatMarkdown(renderfunc RendererFunc, username, botname string) (string, error) {
	if cm.Aux == nil {
		if cm.Equals(llm.Message{}) && cm.From != "" {
			return fmt.Sprintf("(empty from %s)", cm.From), nil
		}
		if cm.Equals(llm.Message{}) {
			return "(empty)", nil
		}
	}
	//slog.Info("formatting", "msg", len(cm.String()))
	var str strings.Builder
	err := cm.FormatMarkdownTo(&str, renderfunc, username, botname)
	if err != nil {
		return str.String(), err
	}
	return str.String(), nil
}

func (cm ChatMessage) FormatMarkdownTo(w io.Writer, renderfunc RendererFunc, username string, botname string) error {
	var err error
	var content string
	switch {
	case cm.Aux != nil:
		switch v := cm.Aux.(type) {
		case error:
			content = fmt.Sprintf("<span style=\"color:red\">⚠️ Error: %s</span>\n", v.Error())
		case string: // e.g., Notes
			content = fmt.Sprintf("\n\n*Note: %s*\n", v)
		default:
			content = fmt.Sprintf("Aux %T: %v", v, v)
		}
	case strings.HasPrefix(cm.Content, "```json\n"):
		//	slog.Info("detected json", "msg", cm.Content)
		content = cm.Content + "\n\n(JSON ACTUAL FENCED)\n"
	case cm.Content != "" && cm.Content[0] == '{':
		//	slog.Info("detected json", "msg", cm.Content)
		content = cm.Content + "\n\n(JSON ACTUAL)\n"
	case ShowThoughts && cm.Content == "" && cm.Thinking != "":
		content = ""
	case cm.Role == "tool" && ShowToolOutputs:
		content = cm.Content
		content = fmt.Sprintf("### tool %q call output\n```\n", cm.ToolName) + content + "\n```\n"
	case cm.Role == "tool":
		content = fmt.Sprintf("### tool %q call output (%d bytes)\n", cm.ToolName, len(cm.Content))
	case cm.Content == "" && !ShowThoughts && len(cm.ToolCalls) != 0:
		content = ""
	case cm.Content != "":
		content = cm.Content
	case len(cm.ToolCalls) == 1:
		content = "Tool Call"
	case len(cm.ToolCalls) != 0:
		content = fmt.Sprintf("Tool Calls: %d", len(cm.ToolCalls))

	default:
		content = "UNKNOWN"
	}
	if ShowThoughts && cm.Thinking != "" {
		content = strings.Join([]string{italicMarkdown(cm.Thinking), content}, "\n\n")
	}
	if len(cm.ToolCalls) != 0 {
		content += fmt.Sprintf("\n```json\n%s\n```\n", toJSONIndent(cm.ToolCalls))
	}
	content = strings.TrimSpace(content)
	//if cm.Role == "tool" {
	//	content = "\n```markdown\n" + content + "\n```\n"
	//}
	if content != "" && renderfunc != nil {
		//		content += "\n\n---\n"
		content, err = renderfunc(content, cm.GetMessageStyle())
		if err != nil {
			return err
		}
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	name := cm.GetName(username, botname)
	if name == "" {
		_, err := w.Write([]byte(content))
		return err
	}
	_, err = fmt.Fprintf(w, "%s: %s", name, content)
	if err != nil {
		return err
	}
	return nil
}

func italicMarkdown(s string) string {
	if s == "" {
		return ""
	}

	// Split the input into individual lines
	lines := strings.Split(s, "\n")

	// Process each line
	for i, line := range lines {
		// Only italicize if the line actually contains text
		if strings.TrimSpace(line) != "" {
			lines[i] = "*" + line + "*"
		}
	}

	// Rejoin the lines back with newlines
	return strings.Join(lines, "\n")
}

func toJSONIndent(v any) string {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		panic(err.Error())
	}
	return string(b)
}

func (cm ChatMessage) GetMessageStyle() int {
	if cm.Aux != nil {
		return 1
	}
	if cm.Content == "" && len(cm.ToolCalls) == 0 {
		return 1
	}
	return 0
}

func (cm ChatMessage) GetName(username, botname string) string {
	name := strings.ToUpper(cm.Role)
	if cm.From != "" {
		name = cm.From
	}
	if cm.Role == "user" && username != "" {
		name = username
	}
	if cm.Role == "assistant" && botname != "" {
		name = botname
	}
	return name
}
