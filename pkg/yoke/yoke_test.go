package yoke_test

import (
	"testing"

	"github.com/aerth/orchard/pkg/yoke"
)

func TestYoke(t *testing.T) {
	t.Run("should json", func(t *testing.T) {
		x := yoke.New()
		x.ShuffleEnums()
		x.WithStringEnum("color", "yellow", "black")
		{
			js := string(x.JSON())
			println(string(js))
			if string(js) != `{"type":"object","properties":{"color":{"type":"string","enum":["yellow","black"]}}}` {
				t.Fatalf("did not json: %s", string(js))
			}
		}
		{
			yoke.AlwaysListType = true
			js := string(x.JSON())
			yoke.AlwaysListType = false
			println(string(js))
			if string(js) != `{"type":"object","properties":{"color":{"type":["string"],"enum":["yellow","black"]}}}` {
				t.Fatalf("did not json: %s", string(js))
			}
		}
	})
}
