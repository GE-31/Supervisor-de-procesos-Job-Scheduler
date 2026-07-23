package worker

import (
	"os"
	"sync"
	"time"
)

type Status string

const (
	StatusStarting   Status = "starting"
	StatusRunning    Status = "running"
	StatusProcessing Status = "processing"
	StatusDegraded   Status = "degraded"
	StatusStopping   Status = "stopping"
	StatusStopped    Status = "stopped"
	StatusFailed     Status = "failed"
)

type Snapshot struct {
	Status             Status    `json:"worker_state"`
	PID                int       `json:"pid"`
	StartedAt          time.Time `json:"started_at"`
	UptimeSeconds      int64     `json:"uptime_seconds"`
	OrdersAPIConnected bool      `json:"orders_api_connected"`
	ActiveJobs         int       `json:"active_jobs"`
	ProcessedOrders    int64     `json:"processed_orders"`
	FailedOrders       int64     `json:"failed_orders"`
	APIFailures        int64     `json:"api_failures"`
	Reconnections      int64     `json:"reconnections"`
	LastQuery          time.Time `json:"last_query,omitempty"`
	LastProcessing     time.Time `json:"last_processing,omitempty"`
	LastError          string    `json:"last_error"`
}
type State struct {
	mu       sync.RWMutex
	snapshot Snapshot
}

func NewState() *State {
	return &State{snapshot: Snapshot{Status: StatusStarting, PID: os.Getpid(), StartedAt: time.Now()}}
}
func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := s.snapshot
	v.UptimeSeconds = int64(time.Since(v.StartedAt).Seconds())
	return v
}
func (s *State) setStatus(status Status) { s.mu.Lock(); s.snapshot.Status = status; s.mu.Unlock() }
func (s *State) apiSuccess() {
	s.mu.Lock()
	if !s.snapshot.OrdersAPIConnected && s.snapshot.APIFailures > 0 {
		s.snapshot.Reconnections++
	}
	s.snapshot.OrdersAPIConnected = true
	s.snapshot.LastQuery = time.Now()
	s.snapshot.LastError = ""
	if s.snapshot.ActiveJobs > 0 {
		s.snapshot.Status = StatusProcessing
	} else {
		s.snapshot.Status = StatusRunning
	}
	s.mu.Unlock()
}
func (s *State) apiFailure(err error) {
	s.mu.Lock()
	s.snapshot.OrdersAPIConnected = false
	s.snapshot.APIFailures++
	s.snapshot.LastQuery = time.Now()
	s.snapshot.LastError = err.Error()
	s.snapshot.Status = StatusDegraded
	s.mu.Unlock()
}
func (s *State) startedJob() {
	s.mu.Lock()
	s.snapshot.ActiveJobs++
	s.snapshot.Status = StatusProcessing
	s.mu.Unlock()
}
func (s *State) finishedJob(err error) {
	s.mu.Lock()
	s.snapshot.ActiveJobs--
	s.snapshot.LastProcessing = time.Now()
	if err != nil {
		s.snapshot.FailedOrders++
		s.snapshot.LastError = err.Error()
	} else {
		s.snapshot.ProcessedOrders++
	}
	if s.snapshot.ActiveJobs == 0 && s.snapshot.OrdersAPIConnected {
		s.snapshot.Status = StatusRunning
	}
	s.mu.Unlock()
}
