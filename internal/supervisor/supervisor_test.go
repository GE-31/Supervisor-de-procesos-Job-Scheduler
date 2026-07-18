package supervisor

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type discardStore struct{}

func (discardStore) Writer(string, string) (io.WriteCloser, error) {
	return nopCloser{Writer: io.Discard}, nil
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func newTestSupervisor(t *testing.T, spec JobSpec) *Supervisor {
	t.Helper()
	s, err := New([]JobSpec{spec}, Backoff{Base: 5 * time.Millisecond, Factor: 2, Max: 20 * time.Millisecond}, 50*time.Millisecond, discardStore{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func waitState(t *testing.T, s *Supervisor, name string, timeout time.Duration, states ...State) Snapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := s.Job(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, state := range states {
			if snap.State == state {
				return snap
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	snap, _ := s.Job(name)
	t.Fatalf("estado final %s, esperaba %v", snap.State, states)
	return Snapshot{}
}

func TestBackoff(t *testing.T) {
	b := Backoff{Base: 10 * time.Millisecond, Factor: 2, Max: 25 * time.Millisecond}
	cases := []struct {
		retry int
		want  time.Duration
	}{{1, 10 * time.Millisecond}, {2, 20 * time.Millisecond}, {3, 25 * time.Millisecond}}
	for _, tc := range cases {
		if got := b.Duration(tc.retry); got != tc.want {
			t.Errorf("retry %d: %s != %s", tc.retry, got, tc.want)
		}
	}
}
func TestNeverSuccess(t *testing.T) {
	s := newTestSupervisor(t, JobSpec{Name: "ok", Command: "sh", Args: []string{"-c", "exit 0"}, Restart: "never", WorkDir: ".", MaxRetries: 3})
	if err := s.StartJob("ok"); err != nil {
		t.Fatal(err)
	}
	snap := waitState(t, s, "ok", time.Second, StateStopped)
	if snap.Retries != 0 {
		t.Fatalf("reintentos=%d", snap.Retries)
	}
}
func TestNeverFailure(t *testing.T) {
	s := newTestSupervisor(t, JobSpec{Name: "fail", Command: "sh", Args: []string{"-c", "exit 1"}, Restart: "never", WorkDir: ".", MaxRetries: 3})
	_ = s.StartJob("fail")
	snap := waitState(t, s, "fail", time.Second, StateFailed)
	if snap.LastError == "" {
		t.Fatal("se esperaba error")
	}
}
func TestOnFailureAndRetryLimit(t *testing.T) {
	s := newTestSupervisor(t, JobSpec{Name: "fail", Command: "sh", Args: []string{"-c", "exit 1"}, Restart: "on-failure", WorkDir: ".", MaxRetries: 2})
	_ = s.StartJob("fail")
	snap := waitState(t, s, "fail", time.Second, StateFailed)
	if snap.Retries != 2 {
		t.Fatalf("reintentos=%d", snap.Retries)
	}
}
func TestAlwaysRestartsSuccess(t *testing.T) {
	s := newTestSupervisor(t, JobSpec{Name: "always", Command: "sh", Args: []string{"-c", "exit 0"}, Restart: "always", WorkDir: ".", MaxRetries: 1})
	_ = s.StartJob("always")
	snap := waitState(t, s, "always", time.Second, StateFailed)
	if snap.Retries != 1 {
		t.Fatalf("reintentos=%d", snap.Retries)
	}
}
func TestStopRestartAndShutdown(t *testing.T) {
	script := filepath.Join(t.TempDir(), "loop.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrap 'exit 0' TERM\nwhile :; do sleep 1; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	s := newTestSupervisor(t, JobSpec{Name: "loop", Command: script, Restart: "never", WorkDir: ".", MaxRetries: 0})
	_ = s.StartJob("loop")
	waitState(t, s, "loop", time.Second, StateRunning)
	if err := s.StopJob("loop"); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, "loop", time.Second, StateStopped)
	if err := s.RestartJob("loop"); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, "loop", time.Second, StateRunning)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, "loop", time.Second, StateStopped)
}
func TestCancellationDuringBackoff(t *testing.T) {
	s := newTestSupervisor(t, JobSpec{Name: "fail", Command: "sh", Args: []string{"-c", "exit 1"}, Restart: "on-failure", WorkDir: ".", MaxRetries: 20})
	_ = s.StartJob("fail")
	waitState(t, s, "fail", time.Second, StateBackoff)
	if err := s.StopJob("fail"); err != nil {
		t.Fatal(err)
	}
	waitState(t, s, "fail", time.Second, StateStopped)
}
