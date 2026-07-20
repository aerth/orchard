// Package testing for testing
package testing

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/aerth/mostly/stackerr"
)

// ScanTools dummy tools from spec (no exec)
func ScanTools(dir string, glob string) ([]json.Marshaler, error) {
	got, err := filepath.Glob(filepath.Join(dir, glob))
	if err != nil {
		return nil, stackerr.Errorf("%w", err)
	}
	a := make([]json.Marshaler, 0, len(got))
	for _, filename := range got {
		js, err := os.ReadFile(filename)
		if err != nil {
			return nil, stackerr.Errorf("%w", err)
		}
		a = append(a, ToolDummy{js: js})
	}
	return a, nil
}

type ToolDummy struct {
	js []byte
}

var _ = []json.Marshaler{ToolDummy{}, &ToolDummy{}}

func (t *ToolDummy) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return stackerr.Errorf("nil b unmarshal")
	}
	if b[0] != '{' {
		return stackerr.Errorf("expected JSON object")
	}
	t.js = b
	return nil
}

func (t ToolDummy) MarshalJSON() ([]byte, error) {
	if t.js == nil {
		return nil, stackerr.Errorf("no underlying json") // since this is a dummy tool, we are NOT returning "null"
	}
	return t.js, nil
}
