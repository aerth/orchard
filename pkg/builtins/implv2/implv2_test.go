package implv2

import (
	"log"
	"testing"
)

func TestOutputParse(t *testing.T) {
	generated := "GOTO func Foo"
	actions, err := OutputParser(generated)
	if err != nil {
		t.Fatal(err)
	}
	for i := range actions {
		log.Printf("actions %d: %#v", i+1, actions[i])
	}
}
