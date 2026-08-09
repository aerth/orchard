package implv2

import (
	"testing"

	"github.com/google/uuid"
)

func TestDiff(t *testing.T) {
	before := `

def greeting():
	print("hello world")

`
	after := `

def greeting():
	print("Hello, World!")

`

	diff, err := Differ(uuid.New(), before, after)
	println(diff)
	if err != nil {
		t.Fatal(err)
	}
}
