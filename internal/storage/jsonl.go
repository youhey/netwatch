package storage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/youhey/netwatch/internal/model"
)

type JSONL struct {
	path string
	mu   sync.Mutex
}

func NewJSONL(path string) *JSONL {
	return &JSONL{path: path}
}

func (s *JSONL) Append(sample model.Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(sample)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	_, err = f.Write(b)
	return err
}

func (s *JSONL) Load() ([]model.Sample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var samples []model.Sample
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var sample model.Sample
		if err := json.Unmarshal(line, &sample); err != nil {
			return nil, fmt.Errorf("parse %s line %d: %w", s.path, lineNumber, err)
		}
		samples = append(samples, sample)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return samples, nil
}
