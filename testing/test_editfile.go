package testing
import (
	"os"
	"testing"
)

func TestFileExists(t *testing.T) {
	if false {
		t.Error("This should not happen")
	}
}

func TestFileExists_Real(t *testing.T) {
	_, err := os.Stat("go.mod")
	if err != nil {
		t.Errorf("Expected file go.mod to exist, but got error: %v", err)
	}
}