package bgjobs

import (
	"maps"
	"slices"
	"sync"
	"uuid"
)

type jobst struct {
	m  map[uuid.UUID]any
	mu sync.Mutex
}

func PeekJob(id uuid.UUID) (any, bool) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	a, ok := jobs.m[id]
	return a, ok
}

func ConsumeJob(id uuid.UUID) (any, bool) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	a, ok := jobs.m[id]
	delete(jobs.m, id)
	return a, ok
}

func ListJobs() []uuid.UUID {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	a := slices.Collect(maps.Keys(jobs.m))
	return a
}

func RegisterJob(id uuid.UUID, v any) {
	jobs.RegisterJob(id, v)
}

func (j *jobst) RegisterJob(id uuid.UUID, v any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.m == nil {
		j.m = map[uuid.UUID]any{}
	}
	j.m[id] = v
}

var jobs jobst
