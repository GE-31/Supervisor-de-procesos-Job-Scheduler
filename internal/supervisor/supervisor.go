package supervisor

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Supervisor struct {
	mu   sync.RWMutex
	jobs map[string]*job
}

func New(specs []JobSpec, backoff Backoff, grace time.Duration, logs OutputStore) (*Supervisor, error) {
	s := &Supervisor{jobs: make(map[string]*job, len(specs))}
	for _, spec := range specs {
		if _, ok := s.jobs[spec.Name]; ok {
			return nil, fmt.Errorf("job duplicado %q", spec.Name)
		}
		s.jobs[spec.Name] = newJob(spec, backoff, grace, logs)
	}
	return s, nil
}

func (s *Supervisor) StartAll() {
	for _, name := range s.names() {
		_ = s.StartJob(name)
	}
}
func (s *Supervisor) StartJob(name string) error {
	j, err := s.find(name)
	if err != nil {
		return err
	}
	return j.start()
}
func (s *Supervisor) StopJob(name string) error {
	j, err := s.find(name)
	if err != nil {
		return err
	}
	return j.stop()
}
func (s *Supervisor) RestartJob(name string) error {
	j, err := s.find(name)
	if err != nil {
		return err
	}
	snap := j.snapshot()
	if snap.State != StateStopped && snap.State != StateFailed {
		if err := j.stop(); err != nil {
			return err
		}
	}
	return j.start()
}
func (s *Supervisor) ListJobs() []Snapshot {
	names := s.names()
	out := make([]Snapshot, 0, len(names))
	for _, name := range names {
		j, _ := s.find(name)
		out = append(out, j.snapshot())
	}
	return out
}
func (s *Supervisor) Job(name string) (Snapshot, error) {
	j, err := s.find(name)
	if err != nil {
		return Snapshot{}, err
	}
	return j.snapshot(), nil
}
func (s *Supervisor) Shutdown(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, name := range s.names() {
		j, _ := s.find(name)
		snap := j.snapshot()
		if snap.State == StateStopped || snap.State == StateFailed {
			continue
		}
		wg.Add(1)
		go func() { defer wg.Done(); _ = j.stop() }()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *Supervisor) find(name string) (*job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	j, ok := s.jobs[name]
	if !ok {
		return nil, ErrNotFound
	}
	return j, nil
}
func (s *Supervisor) names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.jobs))
	for n := range s.jobs {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
