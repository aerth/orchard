package testing

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	rege "m/pkg/regexx"
	"m/pkg/types/chatmessages"
)

func TestImageAttachmentLogic(t *testing.T) {
	// 1. Setup: Create a temporary image file
	tmpDir := t.TempDir()
	imgFile := filepath.Join(tmpDir, "test_image.png")
	imgData := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A} // Minimal dummy header
	err := os.WriteFile(imgFile, imgData, 0o644)
	if err != nil {
		t.Fatalf("failed to create temp image: %v", err)
	}

	// 2. Setup Message
	msg := chatmessages.NewChatMessage("user", fmt.Sprintf("Here is an image: [%s]", imgFile))

	// 3. The Logic (Simulated regex match)
	// matches := []string{imgFile}
	matches := rege.FindImagePaths(msg.Content)

	if len(matches) != 0 {
		msg.Attachments = matches
		for _, filename := range matches {
			img, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("failed to read file: %v", err)
			}
			msg.Images = append(msg.Images, img)
		}
	}

	// 4. Assertions
	if len(msg.Attachments) != 1 {
		t.Errorf("expected 1 attachment, got %d", len(msg.Attachments))
	} else if msg.Attachments[0] != imgFile {
		t.Errorf("expected filename %s, got %s", imgFile, msg.Attachments[0])
	}

	if len(msg.Images) != 1 {
		t.Fatalf("expected 1 image in byte slice, got %d", len(msg.Images))
	} else if !equalBytes(msg.Images[0], imgData) {
		t.Errorf("image bytes mismatch")
	}

	t.Logf("Successfully attached %s (%d bytes)", msg.Attachments[0], msg.Images[0])
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
