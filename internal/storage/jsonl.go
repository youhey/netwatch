package storage

import (
	"bufio"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/youhey/netwatch/internal/model"
)

type JSONL struct {
	path          string
	dataDir       string
	filePattern   string
	retentionDays int
	mu            sync.Mutex
}

func NewJSONL(path string) *JSONL {
	return &JSONL{path: path}
}

func NewRotatingJSONL(dataDir, filePattern string, retentionDays int) *JSONL {
	return &JSONL{
		dataDir:       dataDir,
		filePattern:   filePattern,
		retentionDays: retentionDays,
	}
}

func (s *JSONL) Append(sample model.Sample) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.writePath(time.Now())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if s.dataDir != "" {
		s.cleanupLocked(time.Now())
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
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

	if s.dataDir == "" {
		return s.loadPathLocked(s.path)
	}

	s.cleanupLocked(time.Now())

	paths, err := s.dataFilesLocked()
	if err != nil {
		return nil, err
	}

	var samples []model.Sample
	for _, path := range paths {
		loaded, err := s.loadPathLocked(path)
		if err != nil {
			return nil, err
		}
		samples = append(samples, loaded...)
	}

	return samples, nil
}

func (s *JSONL) loadPathLocked(path string) ([]model.Sample, error) {
	f, err := os.Open(path)
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
			log.Printf("skip invalid JSONL line: path=%s line=%d error=%v", path, lineNumber, err)
			continue
		}
		if sample.Type == "" && sample.Kind != "" {
			sample.Type = sample.Kind
		}
		samples = append(samples, sample)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return samples, nil
}

func (s *JSONL) writePath(now time.Time) string {
	if s.dataDir == "" {
		return s.path
	}
	return filepath.Join(s.dataDir, formatDatePattern(s.filePattern, now))
}

func (s *JSONL) dataFilesLocked() ([]string, error) {
	glob := filepath.Join(s.dataDir, patternGlob(s.filePattern))
	paths, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func (s *JSONL) cleanupLocked(now time.Time) {
	if s.retentionDays <= 0 {
		return
	}
	paths, err := s.dataFilesLocked()
	if err != nil {
		log.Printf("list JSONL files failed: data_dir=%s error=%v", s.dataDir, err)
		return
	}
	cutoff := now.AddDate(0, 0, -s.retentionDays+1)
	for _, path := range paths {
		date, ok := parseDateFromPattern(s.filePattern, filepath.Base(path))
		if !ok || !date.Before(startOfDay(cutoff)) {
			continue
		}
		if err := os.Remove(path); err != nil {
			log.Printf("remove old JSONL failed: path=%s error=%v", path, err)
		}
	}
}

func formatDatePattern(pattern string, now time.Time) string {
	return strings.ReplaceAll(pattern, "%Y-%m-%d", now.Format("2006-01-02"))
}

func patternGlob(pattern string) string {
	return strings.ReplaceAll(pattern, "%Y-%m-%d", "*")
}

func parseDateFromPattern(pattern, filename string) (time.Time, bool) {
	parts := strings.Split(pattern, "%Y-%m-%d")
	if len(parts) != 2 {
		return time.Time{}, false
	}
	if !strings.HasPrefix(filename, parts[0]) || !strings.HasSuffix(filename, parts[1]) {
		return time.Time{}, false
	}
	value := strings.TrimSuffix(strings.TrimPrefix(filename, parts[0]), parts[1])
	date, err := time.Parse("2006-01-02", value)
	return date, err == nil
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
