package conversation

import (
	"time"

	"github.com/google/uuid"
)

// State is used for template
type State struct {
	ParticipantNames []string
	Now              time.Time
	SinceBegin       time.Duration
	SinceRecent      time.Duration
	Task             Task
}
type Task struct {
	ID       uuid.UUID `json:"id"`
	Goal     string    `json:"goal"`
	Steps    []string  `json:"steps,omitempty"`
	Findings []string  `json:"findings,omitempty"`
}
