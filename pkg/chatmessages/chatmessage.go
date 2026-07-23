package chatmessages

import (
	"encoding/json"
	"fmt"
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

type MessageOptions struct {
	Aux         any
	Attachments []string // Attachments filenames
	Ephemeral   bool
}

func (cm ChatMessage) TableName() string {
	return "chat_messages_test"
}

func (cm *ChatMessage) GatherChatResponse(cr llm.ChatResponse) error {
	if cr.Message.Role != "" {
		cm.Role = cr.Message.Role
	}
	cm.Thinking += cr.Message.Thinking
	cm.Content += cr.Message.Content
	if len(cr.Message.ToolCalls) == 0 {
		return nil
	}
	if cm.ToolCallID == "" {
		cm.ToolCallID = cr.Message.ToolCallID
	}
	cm.ToolCalls = append(cm.ToolCalls, cr.Message.ToolCalls...)
	if len(cm.ToolCalls) == 0 {
		cm.ToolCalls = nil
	}
	if cr.Message.Content == " (END)" {
		return fmt.Errorf("broken")
	}
	return nil
}

func (cm ChatMessage) IsEmpty() bool {
	return cm.Thinking == "" && cm.Content == "" && len(cm.ToolCalls) == 0
}

func (cm ChatMessage) String() string {
	return "thr_" + cm.ThreadID.String()[:8] + "_msg_" + cm.MessageID.String()[:8] + "_" + cm.Message.String()
}

type ChatMessages []ChatMessage

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
	l := len(cm) - 1
	if l < n {
		return cm
	}
	return cm[l-n:]
}

func (cm ChatMessages) Top() ChatMessage {
	l := len(cm) - 1
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

func (cm ChatMessages) FormatRoles() []string {
	a := make([]string, len(cm))
	for i := range cm {
		a[i] = cm[i].Role
	}
	return a
}

func (cm ChatMessages) ContainsID(id uuid.UUID) bool {
	return slices.ContainsFunc(cm, func(v ChatMessage) bool {
		return v.MessageID == id
	})
}

type RendererFunc = func(v ChatMessage) string
