package builtintools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aerth/mostly/mllm"
)

// SystemPromptTool allows the agent to modify its own sticky system prompt.
// It can append, prepend, or overwrite the persistent prompt file.
func SystemPromptTool() mllm.Tool {
	return mllm.Tool{
		 truckers.Spec: mllm.Spec(`{
	  "name": "modify_system_prompt",
	  "description": "modifies the persistent sticky system prompt that the agent uses across sessions. Useful for setting long-term preferences or constraints.",
	  "parameters": {
	    "type": "object",
	    "properties": {
	      "action": {
	        "type": "string",
	        "enum": ["append", "prepend", "overwrite"],
	        "description": "The action to take on the sticky prompt"
	      },
	      "content": {
	        "type": "string",
	        "description": "The content to add or set for the sticky prompt"
	      }
	    },
	    "required": ["action", "content"]
	  }
	}`),
		Exec: func(ctx context.Context, args map[string]any) (any, error) {
			action, ok := args["action"].(string)
			if !ok {
				return nil, fmt.Errorf("missing action parameter")
			}
			content, ok := args["content"].(string)
			if !ok {
				return nil, fmt.Errorf("missing content parameter")
			}

			home, _ := os.UserHomeDir()
			path := filepath.Join(home, ".orchard", "sticky_prompt.md")

			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory: %w", err)
			}

			var currentContent string
			if _, err := os.Stat(path); err == nil {
				currentContent, _ = os.ReadFile(path)
			}

			var newContent string
			switch action {
			case "append":
				newContent = string(currentContent) + "\n" + content
			case "prepend":
				newContent = content + "\n" + string(currentContent)
			case "overwrite":
				newContent = content
			default:
				return nil, fmt.Errorf("invalid action: %s", action)
			}

			if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
				return nil, fmt.Errorf("failed to write sticky prompt: %w", err)
			}

			return fmt.Sprintf("Successfully updated sticky prompt with action: %s", action), nil
		},
	}
}
