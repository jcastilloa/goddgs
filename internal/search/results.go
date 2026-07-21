package search

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jcastillo/goddgs/internal/engine"
)

var errEmptyCacheFields = errors.New("At least one cache_field must be provided")

var errUnknownResultCategory = engine.ErrUnknownResultCategory

// Field is one source result field in its declared insertion order.
type Field = engine.Field

// Result is a lossless ordered source result used before conversion to the
// public raw map representation.
type Result = engine.Result

// NewResult constructs a source result from fields in source insertion order.
func NewResult(fields []Field) (Result, error) {
	return engine.NewResult(fields)
}

// NewCategoryResult constructs one frozen source category result shape.
func NewCategoryResult(category string, updates []Field) (Result, error) {
	return engine.NewCategoryResult(category, updates)
}

// ResultsAggregator mirrors Python ResultsAggregator for ordered raw results.
type ResultsAggregator struct {
	cacheFields map[string]struct{}
	entries     map[string]*aggregationEntry
	keyOrder    []string
}

type aggregationEntry struct {
	result Result
	count  int
}

// NewResultsAggregator constructs an aggregator using source cache field names.
func NewResultsAggregator(cacheFields []string) (*ResultsAggregator, error) {
	if len(cacheFields) == 0 {
		return nil, errEmptyCacheFields
	}

	fields := make(map[string]struct{}, len(cacheFields))
	for _, field := range cacheFields {
		fields[field] = struct{}{}
	}
	return &ResultsAggregator{
		cacheFields: fields,
		entries:     make(map[string]*aggregationEntry),
	}, nil
}

// Append records one result occurrence and replaces a duplicate only when its
// source body has more Unicode code points than the cached body.
func (a *ResultsAggregator) Append(result Result) error {
	key, err := a.key(result)
	if err != nil {
		return err
	}

	entry, exists := a.entries[key]
	if !exists {
		a.entries[key] = &aggregationEntry{result: result, count: 1}
		a.keyOrder = append(a.keyOrder, key)
		return nil
	}

	incomingLength, err := resultBodyLength(result)
	if err != nil {
		return err
	}
	cachedLength, err := resultBodyLength(entry.result)
	if err != nil {
		return err
	}
	if incomingLength > cachedLength {
		entry.result = result
	}
	entry.count++
	return nil
}

// Len returns the number of distinct cached result keys.
func (a *ResultsAggregator) Len() int {
	return len(a.entries)
}

// Extract returns cached results by descending occurrence count, keeping first
// encounter order for equal counts.
func (a *ResultsAggregator) Extract() []Result {
	entries := make([]*aggregationEntry, len(a.keyOrder))
	for index, key := range a.keyOrder {
		entries[index] = a.entries[key]
	}
	sort.SliceStable(entries, func(left, right int) bool {
		return entries[left].count > entries[right].count
	})

	results := make([]Result, len(entries))
	for index, entry := range entries {
		results[index] = entry.result
	}
	return results
}

func (a *ResultsAggregator) key(result Result) (string, error) {
	for _, field := range result.Fields() {
		if _, ok := a.cacheFields[field.Name]; ok {
			return sourceString(field.Value)
		}
	}
	return "", fmt.Errorf("item has none of the cache fields")
}

func resultBodyLength(result Result) (int, error) {
	value, ok := result.Value("body")
	if !ok {
		return 0, nil
	}
	switch value := value.(type) {
	case string:
		return utf8.RuneCountInString(value), nil
	case []byte:
		return len(value), nil
	}

	if value == nil {
		return 0, sourceLengthError("NoneType")
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice:
		return reflected.Len(), nil
	default:
		return 0, sourceLengthError(sourceTypeName(value))
	}
}

func sourceLengthError(typeName string) error {
	return errors.New("object of type '" + typeName + "' has no len()")
}

func sourceTypeName(value any) string {
	switch value := value.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case []any:
		return "list"
	case map[string]any:
		return "dict"
	case json.Number:
		if strings.ContainsAny(value.String(), ".eE") {
			return "float"
		}
		return "int"
	case float32, float64:
		return "float"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "int"
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Array, reflect.Slice:
		return "list"
	case reflect.Map:
		return "dict"
	}
	return reflected.Type().String()
}

func sourceString(value any) (string, error) {
	switch value := value.(type) {
	case nil:
		return "None", nil
	case string:
		return value, nil
	case bool:
		if value {
			return "True", nil
		}
		return "False", nil
	case json.Number:
		return value.String(), nil
	case int:
		return strconv.Itoa(value), nil
	case int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(value), nil
	case float32:
		return strconv.FormatFloat(float64(value), 'g', -1, 32), nil
	case float64:
		return strconv.FormatFloat(value, 'g', -1, 64), nil
	case []any:
		if len(value) == 0 {
			return "[]", nil
		}
	case map[string]any:
		if len(value) == 0 {
			return "{}", nil
		}
	}
	return "", fmt.Errorf("cannot convert source cache field value %T to string", value)
}
