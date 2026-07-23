package dto

import (
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
	"strings"
	"time"
)

type JobResponse struct {
	Name          string           `json:"name"`
	Command       string           `json:"command"`
	State         supervisor.State `json:"state"`
	PID           int              `json:"pid"`
	RestartPolicy string           `json:"restart_policy"`
	Retries       int              `json:"retries"`
	MaxRetries    int              `json:"max_retries"`
	StartedAt     *time.Time       `json:"started_at"`
	UptimeSeconds int64            `json:"uptime_seconds"`
	LastError     string           `json:"last_error"`
}

func FromSnapshot(j supervisor.Snapshot) JobResponse {
	var started *time.Time
	var uptime int64
	if !j.StartedAt.IsZero() {
		v := j.StartedAt
		started = &v
		if j.State == supervisor.StateRunning {
			uptime = int64(time.Since(v).Seconds())
			if uptime < 0 {
				uptime = 0
			}
		}
	}
	return JobResponse{Name: j.Name, Command: strings.Join(append([]string{j.Command}, j.Args...), " "), State: j.State, PID: j.PID, RestartPolicy: j.RestartPolicy, Retries: j.Retries, MaxRetries: j.MaxRetries, StartedAt: started, UptimeSeconds: uptime, LastError: j.LastError}
}

type Summary struct {
	Total         int `json:"total"`
	Running       int `json:"running"`
	Backoff       int `json:"backoff"`
	Stopped       int `json:"stopped"`
	Failed        int `json:"failed"`
	TotalRestarts int `json:"total_restarts"`
}

// SummaryFromSnapshots agrega los contadores a partir de la lista completa
// de jobs. La usan tanto el handler HTTP /api/summary como el hub de
// WebSocket, para no calcular el resumen de dos formas distintas.
func SummaryFromSnapshots(snaps []supervisor.Snapshot) Summary {
	summary := Summary{}
	for _, j := range snaps {
		summary.Total++
		summary.TotalRestarts += j.Retries
		switch j.State {
		case supervisor.StateRunning:
			summary.Running++
		case supervisor.StateBackoff:
			summary.Backoff++
		case supervisor.StateStopped:
			summary.Stopped++
		case supervisor.StateFailed:
			summary.Failed++
		}
	}
	return summary
}
