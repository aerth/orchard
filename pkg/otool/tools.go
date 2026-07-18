package otool

import (
	"context"
	"encoding/json"
	"fmt"

	"m/pkg/chatmessages"
	"m/pkg/llm"

	"github.com/aerth/mostly/stackerr"
)

type Tools []Tool

func (t Tools) ToMarshaler() []json.Marshaler {
	a := make([]json.Marshaler, len(t))
	for i := range t {
		a[i] = t[i]
	}
	return a
}

func (t *Tools) FromMarshalers(a []json.Marshaler) error {
	x := make(Tools, len(a))
	for i := range a {
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
	Name string          `json:"name"`
	Spec json.RawMessage // spec also contains name
	exec ExecFunc
}

func (t *Tool) SetExec(fn ExecFunc) {
	t.exec = fn
}

func (t Tool) Exec(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error) {
	if t.exec != nil {
		return t.exec(ctx, caller, toolCallIndex)
	}
	return nil, fmt.Errorf("exec func was called but is not implemented for this tool %q", t.Name)
}

type ExecFunc = func(ctx context.Context, caller llm.Message, toolCallIndex int) (chatmessages.ChatMessages, error)

func (t Tool) MarshalJSON() ([]byte, error) {
	return t.Spec, nil
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
