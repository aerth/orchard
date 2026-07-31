package otool

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"m/pkg/chatmessages"
	"m/pkg/llm"

	"github.com/aerth/mostly/stackerr"
)

type (
	ToolRunnerFunc = func(ctx context.Context, tools Tools, caller chatmessages.ChatMessage) (chatmessages.ChatMessages, error)
	Tools          []Tool
)

// ToMarshaler for api requests
func (t Tools) ToMarshaler() []json.Marshaler {
	a := make([]json.Marshaler, len(t))
	for i := range t {
		a[i] = t[i]
	}
	return a
}

// FromMarshalers is not from random type it is from Tools
func (t *Tools) FromMarshalers(a []json.Marshaler) error {
	x := make(Tools, len(a))
	for i := range a {
		xx, ok := a[i].(Tool)
		if ok {
			x[i] = xx
			continue
		}

		// TODO: check if this part works for a otool.ToolV2 or something

		tn := fmt.Sprintf("%T", a[i])
		if tn != "main.ToolDummy" {
			slog.Error("weird tool", "x", tn)
		}
		b, err := a[i].MarshalJSON()
		if err != nil {
			return stackerr.Errorf("%w", err)
		}
		if err := x[i].UnmarshalJSON(b); err != nil {
			return stackerr.Errorf("%w", err)
		}
	}
	*t = x
	return nil
}

type Tool struct {
	Name string          `json:"name"`     // spec also contains name
	Spec json.RawMessage `json:"function"` // spec also contains name
	exec ExecFunc
}

func (t Tool) GetName() string {
	return t.Name
}

func (t Tools) Names() []string {
	a := make([]string, len(t))
	for i := range t {
		a[i] = t[i].Name
	}
	return a
}

func (t Tool) WithExec(execFn ExecFunc) Tool {
	t.exec = execFn
	return t
}

func (t Tool) GetDescription() string {
	dd := struct {
		Description string `json:"description"`
	}{}
	_ = json.Unmarshal(t.Spec, &dd)
	return dd.Description
}

// Exec is likely a [toolboxes.StaticExecFuncFS]
func (t Tool) Exec(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	if t.exec != nil {
		slog.Debug("exec", "tool", t.Name, "caller_funcs", caller.ToolCalls.GetNames())
		return t.exec(ctx, caller, toolCallIndex)
	}
	return nil, fmt.Errorf("exec func was called but is not implemented for this tool %q", t.Name)
}

type ExecFunc = func(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error)

func (t Tool) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(map[string]any{"type": "function", "function": t.Spec})
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (t *Tool) UnmarshalJSON(b []byte) error {
	var inter struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(b, &inter); err != nil {
		return err
	}
	if inter.Name == "" {
		return fmt.Errorf("missing 'name' field in tool spec")
	}
	t.Name = inter.Name
	t.Spec = b
	return nil
}
