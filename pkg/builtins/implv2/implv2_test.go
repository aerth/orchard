package implv2

import (
	"log"
	"testing"
)

func TestOutputParser_StrictCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantOp      string
		wantContent string
		wantErr     bool
	}{
		// --- VALID CASES ---
		{
			name:        "Valid NEXT_CHUNK",
			input:       "NEXT_CHUNK",
			wantOp:      OpNextChunk,
			wantContent: "",
			wantErr:     false,
		},
		{
			name:        "Valid PREV_CHUNK",
			input:       "PREV_CHUNK",
			wantOp:      OpPrevChunk,
			wantContent: "",
			wantErr:     false,
		},
		{
			name:        "Valid CANCEL",
			input:       "CANCEL",
			wantOp:      OpCancel,
			wantContent: "",
			wantErr:     false,
		},
		{
			name:        "Valid GOTO with single-word target",
			input:       "GOTO func",
			wantOp:      OpGoTo,
			wantContent: "func",
			wantErr:     false,
		},
		{
			name:        "Valid GOTO preserving interior spacing and code tokens",
			input:       "GOTO     def calculate_total(price,  tax_rate):",
			wantOp:      OpGoTo,
			wantContent: "    def calculate_total(price,  tax_rate):", // Note: The 4 spaces of indentation are preserved intact
			wantErr:     false,
		},
		{
			name:        "Valid LOCK single line",
			input:       "LOCK 101",
			wantOp:      OpLockIn,
			wantContent: "101",
			wantErr:     false,
		},
		{
			name:        "Valid LOCK line range",
			input:       "LOCK 101-103",
			wantOp:      OpLockIn,
			wantContent: "101-103",
			wantErr:     false,
		},

		// --- INVALID CASES (JUNK GENERATION FAILS) ---
		{name: "funk", input: "NEXT_CHUNK ", wantOp: "NEXT_CHUNK", wantErr: false},
		{
			name:    "Empty input",
			input:   "",
			wantErr: true,
		},
		{
			name:    "Whitespace-only input",
			input:   "   \n\t ",
			wantErr: true,
		},
		{
			name:    "Lowercase command should fail",
			input:   "goto func Foo",
			wantErr: true,
		},
		{
			name:    "Leading whitespace before command should fail",
			input:   " GOTO func Foo",
			wantErr: true,
		},
		/*		{
				name:    "Multiple spaces between GOTO and target should fail structural check",
				input:   "GOTO  func Foo", // Two spaces directly after GOTO instead of one separator space
				wantErr: true,
			},*/
		{
			name:    "Command with markdown formatting junk should fail",
			input:   "**GOTO** func Foo",
			wantErr: true,
		},
		{
			name:    "Command wrapped in backticks should fail",
			input:   "`NEXT_CHUNK`",
			wantErr: true,
		},
		{
			name:    "Trailing punctuation after parameter-less command should fail",
			input:   "NEXT_CHUNK.",
			wantErr: true,
		},
		{
			name:    "Parameter-less command with conversational tail should fail",
			input:   "NEXT_CHUNK please",
			wantErr: true,
		},
		{
			name:    "GOTO missing argument entirely should fail",
			input:   "GOTO",
			wantErr: true,
		},
		{
			name:    "GOTO with trailing spaces only should fail content checks",
			input:   "GOTO ",
			wantErr: true,
		},
		{
			name:    "LOCK missing coordinates should fail",
			input:   "LOCK",
			wantErr: true,
		},
		{
			name:    "LOCK with trailing conversational chatter should fail",
			input:   "LOCK 101-103 target matches perfectly",
			wantErr: true,
		},
		{
			name:    "LOCK with trailing punctuation should fail",
			input:   "LOCK 101-103.",
			wantErr: true,
		},
		{
			name:    "Pure conversational text instead of protocol command should fail",
			input:   "I want to jump to the calculate function.",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions, err := OutputParser(tt.input)

			// Validate error presence
			if (err != nil) != tt.wantErr {
				t.Fatalf("OutputParser() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return // Test passed because an error was expected and generated
			}

			// Validate returned slice structure
			if len(actions) != 1 {
				t.Fatalf("expected exactly 1 action, got %d", len(actions))
			}

			got := actions[0]
			if got.Op != tt.wantOp {
				t.Errorf("Op = %q, want %q", got.Op, tt.wantOp)
			}
			if got.Content != tt.wantContent {
				t.Errorf("Content = %q, want %q", got.Content, tt.wantContent)
			}
		})
	}
}

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
