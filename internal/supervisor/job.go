package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

type OutputStore interface {
	Writer(job, stream string) (io.WriteCloser, error)
}

type JobSpec struct {
	Name, Command, Restart, WorkDir string
	Args                            []string
	MaxRetries                      int
}

type job struct {
	mu           sync.RWMutex
	spec         JobSpec
	state        State
	pid, retries int
	startedAt    time.Time
	lastError    string
	cancel       context.CancelFunc
	done         chan struct{}
	backoff      Backoff
	grace        time.Duration
	logs         OutputStore
}

func newJob(spec JobSpec, backoff Backoff, grace time.Duration, logs OutputStore) *job {
	return &job{spec: spec, state: StateStopped, backoff: backoff, grace: grace, logs: logs}
}

func (j *job) snapshot() Snapshot {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return Snapshot{Name: j.spec.Name, Command: j.spec.Command, Args: append([]string(nil), j.spec.Args...), State: j.state, PID: j.pid, RestartPolicy: j.spec.Restart, Retries: j.retries, MaxRetries: j.spec.MaxRetries, StartedAt: j.startedAt, LastError: j.lastError}
}

func (j *job) start() error {
	j.mu.Lock()
	if j.state == StateRunning || j.state == StateStarting || j.state == StateBackoff || j.state == StateStopping {
		j.mu.Unlock()
		return ErrInvalidState
	}
	ctx, cancel := context.WithCancel(context.Background())
	j.cancel, j.done, j.state, j.retries, j.lastError = cancel, make(chan struct{}), StateStarting, 0, ""
	done := j.done
	j.mu.Unlock()
	go func() { defer close(done); j.run(ctx) }()
	return nil
}

func (j *job) stop() error {
	j.mu.Lock()
	if j.state == StateStopped || j.state == StateFailed {
		j.mu.Unlock()
		return ErrInvalidState
	}
	j.state = StateStopping
	cancel, done := j.cancel, j.done
	j.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}

func (j *job) run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			j.setFinished(StateStopped, "")
			return
		}
		j.mu.Lock()
		j.state = StateStarting
		j.pid = 0
		j.mu.Unlock()
		err := j.execute(ctx)
		if ctx.Err() != nil {
			j.setFinished(StateStopped, "")
			return
		}
		failed := err != nil
		j.mu.Lock()
		if err != nil {
			j.lastError = err.Error()
		} else {
			j.lastError = ""
		}
		shouldRestart := j.spec.Restart == "always" || (j.spec.Restart == "on-failure" && failed)
		if !shouldRestart {
			if failed {
				j.state = StateFailed
			} else {
				j.state = StateStopped
			}
			j.pid = 0
			j.mu.Unlock()
			return
		}
		if j.retries >= j.spec.MaxRetries {
			j.state = StateFailed
			j.pid = 0
			if err == nil {
				j.lastError = "límite de reintentos alcanzado"
			}
			j.mu.Unlock()
			return
		}
		j.retries++
		delay := j.backoff.Duration(j.retries)
		j.state, j.pid = StateBackoff, 0
		j.mu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			j.setFinished(StateStopped, "")
			return
		case <-timer.C:
		}
	}
}

func (j *job) execute(ctx context.Context) error {
	cmd := exec.Command(j.spec.Command, j.spec.Args...)
	cmd.Dir = j.spec.WorkDir
	// Un grupo propio permite terminar también los descendientes del proceso.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := j.logs.Writer(j.spec.Name, "stdout")
	if err != nil {
		return fmt.Errorf("abrir stdout: %w", err)
	}
	defer stdout.Close()
	stderr, err := j.logs.Writer(j.spec.Name, "stderr")
	if err != nil {
		return fmt.Errorf("abrir stderr: %w", err)
	}
	defer stderr.Close()
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("iniciar: %w", err)
	}
	j.mu.Lock()
	j.state, j.pid, j.startedAt = StateRunning, cmd.Process.Pid, time.Now()
	j.mu.Unlock()
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case err := <-wait:
		return err
	case <-ctx.Done():
		j.mu.Lock()
		j.state = StateStopping
		j.mu.Unlock()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		t := time.NewTimer(j.grace)
		defer t.Stop()
		select {
		case <-wait:
			return context.Canceled
		case <-t.C:
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-wait
			return context.Canceled
		}
	}
}

func (j *job) setFinished(state State, message string) {
	j.mu.Lock()
	j.state, j.pid, j.lastError = state, 0, message
	j.mu.Unlock()
}

var (
	ErrNotFound     = errors.New("job no encontrado")
	ErrInvalidState = errors.New("acción no válida para el estado actual")
)
