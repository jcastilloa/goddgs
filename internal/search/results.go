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

	"github.com/jcastillo/goddgs/internal/normalize"
)

var errEmptyCacheFields = errors.New("At least one cache_field must be provided")

var errUnknownResultCategory = errors.New("unknown source result category")

// Field is one source result field in its declared insertion order.
//
// Result fields must remain ordered internally because Python's aggregator uses
// the first eligible field it encounters, rather than cache-field set order.
type Field struct {
	Name  string
	Value any
}

// Result is a lossless ordered source result used before conversion to the
// public raw map representation.
type Result struct {
	fields []Field
}

// NewResult constructs a source result from fields in source insertion order.
// It applies BaseResult's named-field normalization only when the value is
// truthy, matching the frozen Python source.
func NewResult(fields []Field) (Result, error) {
	result := Result{fields: make([]Field, 0, len(fields))}
	for _, field := range fields {
		if err := result.set(field); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

func (r *Result) set(field Field) error {
	normalized, err := normalizeResultField(field)
	if err != nil {
		return err
	}
	for index := range r.fields {
		if r.fields[index].Name == normalized.Name {
			r.fields[index].Value = normalized.Value
			return nil
		}
	}
	r.fields = append(r.fields, normalized)
	return nil
}

// NewCategoryResult constructs one of the frozen source category result
// shapes. Updates replace declared defaults in place or append dynamic fields
// in their supplied source order.
func NewCategoryResult(category string, updates []Field) (Result, error) {
	fields, err := categoryResultFields(category)
	if err != nil {
		return Result{}, err
	}

	result, err := NewResult(fields)
	if err != nil {
		return Result{}, err
	}
	for _, update := range updates {
		if err := result.set(update); err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

func categoryResultFields(category string) ([]Field, error) {
	switch category {
	case "text":
		return []Field{{Name: "title", Value: ""}, {Name: "href", Value: ""}, {Name: "body", Value: ""}}, nil
	case "images":
		return []Field{
			{Name: "title", Value: ""},
			{Name: "image", Value: ""},
			{Name: "thumbnail", Value: ""},
			{Name: "url", Value: ""},
			{Name: "height", Value: ""},
			{Name: "width", Value: ""},
			{Name: "source", Value: ""},
		}, nil
	case "news":
		return []Field{
			{Name: "date", Value: ""},
			{Name: "title", Value: ""},
			{Name: "body", Value: ""},
			{Name: "url", Value: ""},
			{Name: "image", Value: ""},
			{Name: "source", Value: ""},
		}, nil
	case "videos":
		return []Field{
			{Name: "title", Value: ""},
			{Name: "content", Value: ""},
			{Name: "description", Value: ""},
			{Name: "duration", Value: ""},
			{Name: "embed_html", Value: ""},
			{Name: "embed_url", Value: ""},
			{Name: "image_token", Value: ""},
			{Name: "images", Value: map[string]any{}},
			{Name: "provider", Value: ""},
			{Name: "published", Value: ""},
			{Name: "publisher", Value: ""},
			{Name: "statistics", Value: map[string]any{}},
			{Name: "uploader", Value: ""},
		}, nil
	case "books":
		return []Field{
			{Name: "title", Value: ""},
			{Name: "author", Value: ""},
			{Name: "publisher", Value: ""},
			{Name: "info", Value: ""},
			{Name: "url", Value: ""},
			{Name: "thumbnail", Value: ""},
		}, nil
	default:
		return nil, fmt.Errorf("%w: %q", errUnknownResultCategory, category)
	}
}

// Fields returns the result fields in source insertion order.
func (r Result) Fields() []Field {
	fields := make([]Field, len(r.fields))
	copy(fields, r.fields)
	return fields
}

// Map returns the result's raw fields and values. Field ordering is retained
// by Result for internal source-compatible operations; Go map iteration has no
// ordering guarantee.
func (r Result) Map() map[string]any {
	values := make(map[string]any, len(r.fields))
	for _, field := range r.fields {
		values[field.Name] = field.Value
	}
	return values
}

func (r Result) value(name string) (any, bool) {
	for _, field := range r.fields {
		if field.Name == name {
			return field.Value, true
		}
	}
	return nil, false
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
	for _, field := range result.fields {
		if _, ok := a.cacheFields[field.Name]; ok {
			return sourceString(field.Value)
		}
	}
	return "", fmt.Errorf("item has none of the cache fields")
}

func normalizeResultField(field Field) (Field, error) {
	if !sourceTruthy(field.Value) {
		return field, nil
	}

	switch field.Name {
	case "title", "body", "author", "publisher", "info":
		value, err := sourceText(field.Name, field.Value)
		if err != nil {
			return Field{}, err
		}
		field.Value = normalize.Text(value)
	case "href", "url", "thumbnail", "image":
		value, err := sourceText(field.Name, field.Value)
		if err != nil {
			return Field{}, err
		}
		field.Value = normalize.URL(value)
	case "date":
		value, err := normalize.Date(sourceDateValue(field.Value))
		if err != nil {
			return Field{}, err
		}
		field.Value = value
	}
	return field, nil
}

func sourceDateValue(value any) any {
	number, ok := value.(json.Number)
	if !ok {
		return value
	}
	integer, err := number.Int64()
	if err != nil {
		return value
	}
	return integer
}

func sourceText(name string, value any) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("cannot normalize source field %q with %T", name, value)
	}
	return text, nil
}

func sourceTruthy(value any) bool {
	if value == nil {
		return false
	}
	if number, ok := value.(json.Number); ok {
		parsed, err := strconv.ParseFloat(number.String(), 64)
		return err != nil || parsed != 0
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return reflected.Len() != 0
	case reflect.Bool:
		return reflected.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflected.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return reflected.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return reflected.Float() != 0
	}
	return true
}

func resultBodyLength(result Result) (int, error) {
	value, ok := result.value("body")
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
