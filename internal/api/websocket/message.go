// Package websocket expone el estado del supervisor al navegador en tiempo
// real. El servidor solo empuja eventos (servidor → navegador); no acepta
// comandos por este canal — iniciar, detener y reiniciar procesos sigue
// yendo por los POST /api/jobs/{name}/{start,stop,restart} existentes.
package websocket

import (
	"time"

	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/api/dto"
	joblog "github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/logging"
	"github.com/GE-31/Supervisor-de-procesos-Job-Scheduler/internal/supervisor"
)

// Event es el mensaje JSON que el hub envía a cada cliente conectado.
// Los campos con omitempty se llenan según Type:
//
//   - "jobs.snapshot": Jobs + Summary — foto completa, enviada justo tras
//     conectar (o reconectar), para que el navegador no dependa del primer
//     ciclo de polling.
//   - "job.<estado>" (job.starting, job.running, job.backoff,
//     job.stopping, job.stopped, job.failed): Job + Summary — una fila
//     cambió; el resumen recalculado viaja en el mismo mensaje para no
//     necesitar un segundo viaje de red.
type Event struct {
	Type      string            `json:"type"`
	Job       *dto.JobResponse  `json:"job,omitempty"`
	Jobs      []dto.JobResponse `json:"jobs,omitempty"`
	Summary   *dto.Summary      `json:"summary,omitempty"`
	LogJob    string            `json:"log_job,omitempty"`
	LogEntry  *joblog.Entry     `json:"log_entry,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// jobEventType traduce el estado real del job en el tipo de evento. No se
// inventan estados adicionales (p. ej. un "restarted" artificial): un
// reinicio ya se ve como la secuencia real stopping → stopped → starting →
// running.
func jobEventType(state supervisor.State) string {
	return "job." + string(state)
}
