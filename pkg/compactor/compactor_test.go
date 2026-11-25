package compactor_test

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/aerth/mostly/mllm"
	"github.com/aerth/orchard"
	"github.com/aerth/orchard/pkg/compactor"
)

//nogo:embed test.convo.big.json
//var testconvo string

func TestCompactor(t *testing.T) {
	cfg := orchard.Config{
		E: mllm.Default(),
		//	mllm.Tools: nil,
	}
	if err := cfg.E.ValidateConfig(false); err != nil {
		t.Skip(err)
		return
	}
	var convo mllm.ChatMessages
	testconvo, err := os.ReadFile("test.convo.big.json")
	if err != nil {
		log.Printf("Skipping compression test: %v", err)
		t.SkipNow()
	}
	err = json.Unmarshal([]byte(testconvo), &convo)
	if err != nil {
		t.Fatal(err)
	}
	mllm.DefaultKeepAlive = 0
	t.Run(fmt.Sprintf("should-immediately-compact-%d", len(convo)), func(t *testing.T) {
		ctx := t.Context()
		compactor.CompactConvoAtLength = 10
		cfg.PreTextHookEditP1 = compactor.V1
		cfg.E.Options.Debug = 2
		ctx = mllm.WithClient(ctx, cfg.E)
		got, err := compactor.V1(ctx, convo)
		if err != nil {
			t.Fatal(err)
		}

		if len(got) >= len(convo) {
			t.Fatal(fmt.Errorf("did not compress"))
		}
		println("got", len(got))
		for _, v := range got {
			println(v.Role, v.Content)
		}
		b, err := json.MarshalIndent(got, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile("convo.tmp.json", b, 0o644)
	})
}
