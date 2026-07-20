package search

import (
	"errors"
	"math/rand/v2"
	"sort"
	"strings"
	"unicode"

	"github.com/jcastillo/goddgs/internal/engine"
)

// BackendSelector preserves frozen Python backend selection independently from
// engine construction and execution.
type BackendSelector struct {
	categories map[string][]engine.Metadata
	shuffle    func([]string)
}

// NewBackendSelector constructs a selector over active metadata. The shuffle
// seam is injected for differential tests; nil uses source-equivalent random
// ordering per selection call.
func NewBackendSelector(categories []engine.Category, shuffle func([]string)) *BackendSelector {
	byName := make(map[string][]engine.Metadata, len(categories))
	for _, category := range categories {
		byName[category.Category] = append([]engine.Metadata(nil), category.Engines...)
	}
	if shuffle == nil {
		shuffle = randomShuffle
	}
	return &BackendSelector{categories: byName, shuffle: shuffle}
}

// Select returns active metadata in frozen Python backend/priority order.
func (s *BackendSelector) Select(category, backend string) ([]engine.Metadata, error) {
	engines, exists := s.categories[category]
	if !exists {
		return nil, sourceKeyError(category)
	}

	keys := make([]string, len(engines))
	byName := make(map[string]engine.Metadata, len(engines))
	for index, metadata := range engines {
		keys[index] = metadata.Name
		byName[metadata.Name] = metadata
	}
	s.shuffle(keys)

	requested := sourceSplitBackend(backend)
	selectionKeys := requested
	if containsBackend(requested, "auto") || containsBackend(requested, "all") {
		selectionKeys = sourceAutomaticKeys(category, keys)
	}

	selection := make([]engine.Metadata, 0, len(selectionKeys))
	for _, key := range selectionKeys {
		metadata, exists := byName[key]
		if exists {
			selection = append(selection, metadata)
		}
	}
	if len(selection) == 0 {
		return s.Select(category, "auto")
	}

	sort.SliceStable(selection, func(left, right int) bool {
		return selection[left].Priority > selection[right].Priority
	})
	return selection, nil
}

func randomShuffle(keys []string) {
	rand.Shuffle(len(keys), func(left, right int) {
		keys[left], keys[right] = keys[right], keys[left]
	})
}

func sourceSplitBackend(backend string) []string {
	parts := strings.Split(backend, ",")
	for index, part := range parts {
		parts[index] = sourceStrip(part)
	}
	return parts
}

func sourceAutomaticKeys(category string, keys []string) []string {
	if category != "text" {
		return keys
	}

	automatic := make([]string, 0, len(keys)+2)
	for _, preferred := range []string{"wikipedia", "grokipedia"} {
		if containsBackend(keys, preferred) {
			automatic = append(automatic, preferred)
		}
	}
	for _, key := range keys {
		if key != "wikipedia" && key != "grokipedia" {
			automatic = append(automatic, key)
		}
	}
	return automatic
}

func containsBackend(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sourceStrip(value string) string {
	return strings.TrimFunc(value, sourceSpace)
}

func sourceSpace(character rune) bool {
	return unicode.IsSpace(character) || '\x1c' <= character && character <= '\x1f'
}

func sourceKeyError(key string) error {
	return errors.New("'" + key + "'")
}
