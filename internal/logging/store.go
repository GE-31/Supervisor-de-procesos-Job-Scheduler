package logging

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
}

type Store struct {
	dir string
	mu  sync.Mutex
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("crear directorio de logs: %w", err)
	}
	return &Store{dir: dir}, nil
}

func validName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, `/\\`)
}

func (s *Store) Writer(job, stream string) (io.WriteCloser, error) {
	if !validName(job) || (stream != "stdout" && stream != "stderr") {
		return nil, fmt.Errorf("nombre de log no válido")
	}
	path := filepath.Join(s.dir, job+"."+stream+".log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	return &lineWriter{file: f, stream: stream, mu: &s.mu}, nil
}

type lineWriter struct {
	file    *os.File
	stream  string
	mu      *sync.Mutex
	pending string
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending += string(p)
	for {
		i := strings.IndexByte(w.pending, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSuffix(w.pending[:i], "\r")
		w.pending = w.pending[i+1:]
		if _, err := fmt.Fprintf(w.file, "%s\t%s\t%s\n", time.Now().Format(time.RFC3339Nano), w.stream, line); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
func (w *lineWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending != "" {
		_, _ = fmt.Fprintf(w.file, "%s\t%s\t%s\n", time.Now().Format(time.RFC3339Nano), w.stream, w.pending)
	}
	return w.file.Close()
}

func (s *Store) Read(job string, limit int) ([]Entry, error) {
	if !validName(job) {
		return nil, fmt.Errorf("nombre de log no válido")
	}
	if limit < 1 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	entries := make([]Entry, 0, limit*2)
	for _, stream := range []string{"stdout", "stderr"} {
		path := filepath.Join(s.dir, job+"."+stream+".log")
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			parts := strings.SplitN(scanner.Text(), "\t", 3)
			if len(parts) != 3 {
				continue
			}
			stamp, err := time.Parse(time.RFC3339Nano, parts[0])
			if err != nil {
				continue
			}
			entries = append(entries, Entry{Timestamp: stamp, Stream: parts[1], Message: parts[2]})
		}
		scanErr := scanner.Err()
		_ = file.Close()
		if scanErr != nil {
			return nil, scanErr
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Timestamp.Before(entries[j].Timestamp) })
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries, nil
}
