package conversation

import (
	"slices"
	"sync"
	"time"

	"uuid"
)

// State is used for template
type State struct {
	Now              time.Time // The current time
	From             string    // From (project_manager)
	Available        []string  // The employees available (snake-cased).
	ParticipantNames []string  // All current participants or mentioned (title cased, eg. Chief,Project Manager)
	SinceBegin       time.Duration
	SinceRecent      time.Duration
	//
	CurrentTask Task // the current task whos pointer may be modified by other goroutines guarded by mu
	mu          sync.Mutex
}

func Init(s *State) {
}

func New() State {
	return State{}
}

func (s *State) Lock() {
	s.mu.Lock()
}

func (s *State) Unlock() {
	s.mu.Unlock()
}

func (s *State) Snapshot() State {
	return State{
		Now:              time.Now().Truncate(time.Second),
		Available:        s.Available,
		From:             s.From,
		ParticipantNames: slices.Clone(s.ParticipantNames),
		SinceBegin:       s.SinceBegin,
		SinceRecent:      s.SinceRecent,
		CurrentTask:      s.CurrentTask,
	}
}

type Task struct {
	ID       uuid.UUID `json:"id"`
	Goal     string    `json:"goal"`
	Steps    []string  `json:"steps,omitempty"`
	Findings []string  `json:"findings,omitempty"`
}
