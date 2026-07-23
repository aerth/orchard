// Package yoke is to harness and limit.
package yoke

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"

	"m/pkg/chat"
	api "m/pkg/llm"
	"m/pkg/orderedmap"

	//	"m/pkg/ollama/api/orderedmap"
	messages "m/pkg/chatmessages"

	"github.com/aerth/mostly/stackerr"
)

type PropertyType []string

func (p PropertyType) MarshalJSON() ([]byte, error) {
	if false && len(p) == 1 {
		return json.Marshal(p[0])
	}
	return json.Marshal([]string(p[:]))
}

func MakeItem(typ string) Property {
	return Property{
		Type: PropertyType{typ},
	}
}

func FormatList(typenames ...string) json.RawMessage {
	var items any
	if len(typenames) == 1 {
		items = MakeItem(typenames[0])
	} else {
		anyOf := make([]Property, len(typenames))
		for i := range typenames {
			anyOf[i] = MakeItem(typenames[i])
		}
		items = map[string]any{"anyOf": anyOf}
	}

	b, err := json.Marshal(map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "array",
		"items":   items,
	})
	if err != nil {
		panic(err.Error())
	}
	return b
}

func FormatObject(propertyMap PropertiesMap, required []string) json.RawMessage {
	req := ToAny(required)
	b, err := json.Marshal(map[string]any{
		"$schema":    "http://json-schema.org/draft-07/schema#",
		"type":       "object",
		"properties": propertyMap,
		"required":   req,
	})
	if err != nil {
		panic(err.Error())
	}
	return b
}

type YR struct {
	Type       string        `json:"type"`
	Properties PropertiesMap `json:"properties"`
	Required   []string      `json:"required,omitempty"`
}
type (
	PropertiesMap = *orderedmap.Map[string, Property] // pointer
	Property      struct {
		// based on api.ToolProperty
		Type        PropertyType  `json:"type,omitempty"`
		Items       any           `json:"items,omitempty"`
		Description string        `json:"description,omitempty"`
		Enum        []any         `json:"enum,omitempty"`
		Properties  PropertiesMap `json:"properties,omitempty"`
		Required    []string      `json:"required,omitempty"`
		Minimum     any           `json:"minimum,omitempty"`
		Maximum     any           `json:"maximum,omitempty"`
	}
)

func (p Property) WithProperty(name string, prop Property) Property {
	p.Properties.Set(name, prop)
	return p
}

func (p Property) WithDescription(s string) Property {
	p.Description = s
	return p
}

func NewPropertiesMap() PropertiesMap {
	x := orderedmap.New[string, Property]()
	return x
}

func (y *YR) JSON() json.RawMessage {
	js, err := json.Marshal(y)
	if err != nil {
		panic(err.Error())
	}
	return js
}

func (y *YR) ShuffleEnums() *YR {
	for k, v := range y.Properties.All() {
		if len(v.Enum) != 0 {
			rand.Shuffle(len(v.Enum), func(i, j int) {
				v.Enum[i], v.Enum[j] = v.Enum[j], v.Enum[i]
			})
			y.Properties.Set(k, v) // TODO: needed?
		}
	}
	return y
}

func (y *YR) Run(ctx context.Context, msgs ...api.Message) (api.Message, error) {
	if len(msgs) == 0 {
		msgs = api.Messages{{Role: "system"}}
	}
	creq := api.NewChatRequest()
	creq.Messages = msgs
	creq.Format = y.JSON()
	got := new(messages.ChatMessage)
	err := chat.ChatReq(ctx, creq, chat.Ext{Handlers: []chat.ChatHandler{got.GatherChatResponse}})
	if err != nil {
		return got.Message, stackerr.Errorf("%w", err)
	}
	if got.Content == "" {
		return got.Message, stackerr.Errorf("yoke: no output")
	}
	if y.Properties.Len() == 0 {
		return got.Message, nil
	}
	if y.Properties.Len() > 1 {
		return got.Message, nil // json direct
	}
	// extract the single param into the raw Content
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(got.Content), &m); err != nil {
		return got.Message, stackerr.Errorf("%w", err)
	}
	if len(m) != 1 {
		return got.Message, stackerr.Errorf("not 1 but %d", len(m))
	}
	for _, s := range m {
		got.Content = string(s)
	}
	return got.Message, nil
}

func (y *YR) WithProperty(name string, property Property) *YR {
	y.Properties.Set(name, property)
	y.Required = append(y.Required, name)
	return y
}

func (y *YR) WithStringEnum(name string, v ...string) *YR {
	y.Properties.Set(name, NewStringEnum(v...))
	return y
}

func (y *YR) WithAnyEnum(name string, v ...any) *YR {
	y.Properties.Set(name, NewAnyEnum(v...))
	return y
}

func NewYoke[T any](name string, v ...T) *YR {
	switch a := any(v).(type) {
	case []string:
		return New().WithProperty(name, NewStringEnum(a...))
	case []int:
		return New().WithProperty(name, NewIntEnum(a...))
	case []bool:
		return New().WithProperty(name, NewBoolEnum())
	default:
		panic(fmt.Sprintf("not string int bool but %T", v))
	}
}

func New() *YR {
	return &YR{Type: "object", Properties: NewPropertiesMap(), Required: nil}
}

func (y *YR) AskYesNo(ctx context.Context, msgs api.Messages, question string) (*bool, error) {
	if y.Properties.Len() == 0 {
		y.Properties.Set("value", NewBoolEnum())
	}
	msgs = append(msgs, api.Message{Role: "system", Content: question})
	a, err := y.Run(ctx, msgs...)
	if err != nil {
		return nil, err
	}
	wrap := struct {
		Value *bool `json:"value"`
	}{}
	if strings.HasPrefix(a.Content, "{") {
		if err := json.Unmarshal([]byte(a.Content), &wrap); err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal([]byte(a.Content), &wrap.Value); err != nil {
			return nil, err
		}
	}
	return wrap.Value, nil
}

func (y *YR) WithMap(x PropertiesMap) *YR {
	for k, v := range x.All() {
		y.Properties.Set(k, v)
	}
	return y
}

// NewStringArray generates an array of strings, such as target_filenames or target_languages
func NewStringArray() Property {
	return Property{
		Type:  PropertyType{"array"},
		Items: map[string]any{"type": "string"},
	}
}

func NewNumberArray() Property {
	return Property{
		Type:  PropertyType{"array"},
		Items: map[string]any{"type": "number"},
	}
}

// NewStringEnum generates only one of these available
func NewStringEnum(vals ...string) Property {
	return Property{
		Type: PropertyType{"string"},
		Enum: ToAny(vals),
	}
}

func NewString() Property {
	return NewStringEnum()
}

func NewOptionalString() Property {
	x := NewStringEnum()
	x.Description += " (optional)"
	return x
}

// NewBoolEnum generates only one of these available
func NewBoolEnum() Property {
	return Property{
		Type: PropertyType{"boolean"},
		Enum: []any{true, false},
	}
}

// NewIntEnum generates only one of these available
func NewIntEnum(vals ...int) Property {
	return Property{
		Type: PropertyType{"number"},
		Enum: ToAny(vals),
	}
}

// NewAnyEnum generates only one of these available (NOTE: assumes string-parsable)
func NewAnyEnum(vals ...any) Property {
	return Property{
		Type: PropertyType{"string"},
		Enum: vals,
	}
}

func ToAny[T any](x []T) []any {
	if len(x) == 0 {
		return nil // important
	}
	a := make([]any, len(x))
	for i := range a {
		a[i] = x[i]
	}
	return a
}

func NewNumberRange[T cmp.Ordered](minNum, maxNum T) Property {
	return Property{
		Type:    PropertyType{"number"},
		Minimum: minNum,
		Maximum: maxNum,
	}
}

func NewFloatRange() Property {
	return Property{}
}
