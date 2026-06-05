package collector

import (
	"sort"
	"sync"
	"time"

	"github.com/youhey/netwatch/internal/model"
)

type State struct {
	mu     sync.RWMutex
	latest map[string]model.Sample
	series []model.Sample
}

func NewState() *State {
	return &State{
		latest: make(map[string]model.Sample),
	}
}

func (s *State) Load(samples []model.Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.series = append(s.series, samples...)
	sort.SliceStable(s.series, func(i, j int) bool {
		return s.series[i].Timestamp.Before(s.series[j].Timestamp)
	})
	for _, sample := range s.series {
		s.latest[sample.Name] = sample
	}
}

func (s *State) Add(sample model.Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.series = append(s.series, sample)
	s.latest[sample.Name] = sample
}

func (s *State) LatestAll() []model.Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	samples := make([]model.Sample, 0, len(s.latest))
	for _, sample := range s.latest {
		samples = append(samples, sample)
	}
	sortSamples(samples)

	return samples
}

func (s *State) LatestByType(sampleType string) []model.Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var samples []model.Sample
	for _, sample := range s.latest {
		if sample.Type == sampleType {
			samples = append(samples, sample)
		}
	}
	sortSamples(samples)

	return samples
}

func (s *State) Series(name string, since time.Time) []model.Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var samples []model.Sample
	for _, sample := range s.series {
		if sample.Name == name && !sample.Timestamp.Before(since) {
			samples = append(samples, sample)
		}
	}
	sortSamples(samples)

	return samples
}

func (s *State) SeriesByType(sampleType, name string, since time.Time) []model.Sample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var samples []model.Sample
	for _, sample := range s.series {
		if sample.Type == sampleType && sample.Name == name && !sample.Timestamp.Before(since) {
			samples = append(samples, sample)
		}
	}
	sortSamples(samples)

	return samples
}

func sortSamples(samples []model.Sample) {
	sort.SliceStable(samples, func(i, j int) bool {
		if samples[i].Name == samples[j].Name {
			return samples[i].Timestamp.Before(samples[j].Timestamp)
		}
		return samples[i].Name < samples[j].Name
	})
}
