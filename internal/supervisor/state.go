package supervisor

import "time"

type State string

const (
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateBackoff  State = "backoff"
	StateStopping State = "stopping"
	StateStopped  State = "stopped"
	StateFailed   State = "failed"
)

type Snapshot struct {
	Name          string
	Command       string
	Args          []string
	State         State
	PID           int
	RestartPolicy string
	Retries       int
	MaxRetries    int
	StartedAt     time.Time
	LastError     string
}
