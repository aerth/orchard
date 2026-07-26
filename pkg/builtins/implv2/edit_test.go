package implv2

import (
	"reflect"
	"testing"
)

func TestSession_ApplySurgicalEdit(t *testing.T) {
	tests := []struct {
		name             string
		initialLines     []string
		lockStart        int
		lockEnd          int
		replacementLines []string
		wantLines        []string
		wantErr          bool
	}{
		{
			name: "Surgically edit a single line",
			initialLines: []string{
				"line 1",
				"line 2: def old_func():",
				"line 3",
			},
			lockStart:        2,
			lockEnd:          2,
			replacementLines: []string{"line 2: def new_func():"},
			wantLines: []string{
				"line 1",
				"line 2: def new_func():",
				"line 3",
			},
			wantErr: false,
		},
		{
			name: "Replace a multi-line block with fewer lines (Shrink)",
			initialLines: []string{
				"101: start",
				"102: remove me 1",
				"103: remove me 2",
				"104: end",
			},
			lockStart:        2,
			lockEnd:          3,
			replacementLines: []string{"102: squeezed into one line"},
			wantLines: []string{
				"101: start",
				"102: squeezed into one line",
				"104: end",
			},
			wantErr: false,
		},
		{
			name: "Replace a single line with multiple lines (Expand)",
			initialLines: []string{
				"import os",
				"def run(): pass",
				"print('done')",
			},
			lockStart: 2,
			lockEnd:   2,
			replacementLines: []string{
				"def run():",
				"    setup_env()",
				"    execute()",
			},
			wantLines: []string{
				"import os",
				"def run():",
				"    setup_env()",
				"    execute()",
				"print('done')",
			},
			wantErr: false,
		},
		{
			name: "Surgically clear/delete lines entirely",
			initialLines: []string{
				"keep prefix",
				"delete this",
				"keep suffix",
			},
			lockStart:        2,
			lockEnd:          2,
			replacementLines: []string{}, // Empty slice clears the target range
			wantLines: []string{
				"keep prefix",
				"keep suffix",
			},
			wantErr: false,
		},
		{
			name: "Edit at the absolute boundary (First line of file)",
			initialLines: []string{
				"old first line",
				"second line",
			},
			lockStart:        1,
			lockEnd:          1,
			replacementLines: []string{"new first line"},
			wantLines: []string{
				"new first line",
				"second line",
			},
			wantErr: false,
		},
		{
			name: "Edit at the absolute boundary (Last line of file)",
			initialLines: []string{
				"first line",
				"old last line",
			},
			lockStart:        2,
			lockEnd:          2,
			replacementLines: []string{"new last line"},
			wantLines: []string{
				"first line",
				"new last line",
			},
			wantErr: false,
		},

		// --- ERROR TRAPPING CASES ---
		{
			name:             "Fail when no lock is active",
			initialLines:     []string{"one", "two"},
			lockStart:        0,
			lockEnd:          0,
			replacementLines: []string{"error"},
			wantLines:        []string{"one", "two"},
			wantErr:          true,
		},
		{
			name:             "Fail on out-of-bounds index (Lock past file size)",
			initialLines:     []string{"one", "two"},
			lockStart:        2,
			lockEnd:          5,
			replacementLines: []string{"error"},
			wantLines:        []string{"one", "two"},
			wantErr:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{
				Lines:     tt.initialLines,
				LockStart: tt.lockStart,
				LockEnd:   tt.lockEnd,
			}

			err := s.ApplySurgicalEdit(tt.replacementLines)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ApplySurgicalEdit() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				return
			}

			// Verify array mutations match expectations exactly
			if !reflect.DeepEqual(s.Lines, tt.wantLines) {
				t.Errorf("Result lines mismatch!\ngot:  %q\nwant: %q", s.Lines, tt.wantLines)
			}

			// Verify status flags automatically clear out after application
			if s.LockStart != 0 || s.LockEnd != 0 {
				t.Errorf("State error: lock ranges failed to clear after mutation. Got bounds: %d-%d", s.LockStart, s.LockEnd)
			}
		})
	}
}
