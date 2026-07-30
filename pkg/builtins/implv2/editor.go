package implv2

import (
	"errors"
	"slices"

	"github.com/aerth/mostly/stackerr"
)

var ErrKeywordCancelled = errors.New("contains CANCEL")

// ApplySurgicalEdit swaps out the locked coordinate range in memory.
// It maps 1-based human line numbers directly to 0-based Go array indices.
//
// If error is nil, caller should handle reply messages and writing files.
func (s *Session) ApplySurgicalEdit(replacementLines []string) error {
	if i := slices.Index(replacementLines, "CANCEL"); i >= 0 {
		return stackerr.Errorf("%w", ErrKeywordCancelled)
	}
	if s.LockStart == 0 || s.LockEnd == 0 {
		return stackerr.Errorf("state error: cannot apply edit without an active coordinate lock")
	}

	// 1. Calculate slice bounds
	// Example: lines 101-103 means indexing from 100 up to (but excluding) 103
	startIdx := s.LockStart - 1
	endIdx := s.LockEnd

	if startIdx < 0 || endIdx > len(s.Lines) {
		return stackerr.Errorf("slice error: coordinate bounds %d-%d are out of file range", s.LockStart, s.LockEnd)
	}

	// 2. Perform the in-memory surgical splice
	// Combine the prefix slice, new replacement array lines, and the suffix slice
	updatedLines := make([]string, 0, startIdx+len(replacementLines)+(len(s.Lines)-endIdx))
	updatedLines = append(updatedLines, s.Lines[:startIdx]...)
	updatedLines = append(updatedLines, replacementLines...)
	updatedLines = append(updatedLines, s.Lines[endIdx:]...)

	s.Lines = updatedLines

	return nil
}
