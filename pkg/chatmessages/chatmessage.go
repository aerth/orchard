package chatmessages

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"m/pkg/llm"
	"m/pkg/vector256"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (cm ChatMessage) GetID() uint {
	return cm.DBID
}

type ChatMessage struct {
	DBID           uint            `gorm:"primarykey;column:id" json:"id,omitempty"`                                       // The MESSAGE DATABASE ID INTERNAL UNUSED ORDERED
	MessageID      uuid.UUID       `gorm:"type:uuid;primaryKey;notnull;" json:"message_id,omitempty"`                      // MessageID is unique identifier for each message
	ThreadID       uuid.UUID       `gorm:"type:uuid;not null;index:idx_thread_time,priority:1" json:"thread_id,omitempty"` // ThreadID for storing in DB
	CreatedAt      time.Time       `json:"time"`
	UpdatedAt      time.Time       `json:"modified_time"`
	DeletedAt      *gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	llm.Message    `gorm:"embedded" json:"message"`
	Embedding      *vector256.Vector256 `gorm:"type:vector(256);null" json:"-"`
	From           string               `json:"from,omitempty"` // name of assistant
	MessageOptions `gorm:"-" json:"-"`
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
	t := time.Now().UTC()
	return &ChatMessage{Message: m, CreatedAt: t, MessageID: uuid.New(), UpdatedAt: t}
}

func NewChatMessage(role string, content string) *ChatMessage {
	return New(llm.Message{Role: role, Content: content})
}

func NewFrom(from string, m llm.Message) *ChatMessage {
	t := time.Now().UTC()
	return &ChatMessage{From: from, Message: m, CreatedAt: t, MessageID: uuid.New(), UpdatedAt: t}
}

func NewFromEphemeral(from string, m llm.Message) *ChatMessage {
	t := time.Now().UTC()
	return &ChatMessage{From: from, Message: m, CreatedAt: t, MessageID: uuid.New(), UpdatedAt: t, MessageOptions: MessageOptions{Ephemeral: true}}
}

type MessageOptions struct {
	Aux       any
	Ephemeral bool
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
	return "chat-message-" + cm.MessageID.String() + " " + cm.Message.String()
}

type ChatMessages []ChatMessage

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

		slog.Warn("got raw", "i", i, "msg", cm[i].Message)
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
