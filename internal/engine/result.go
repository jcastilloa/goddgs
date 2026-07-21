package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/jcastillo/goddgs/internal/normalize"
)

// ErrUnknownResultCategory reports a category absent from the frozen source
// result dataclasses.
var ErrUnknownResultCategory = errors.New("unknown source result category")

// Field is one source result field in its declared insertion order.
type Field struct {
	Name  string
	Value any
}

// Result is a lossless ordered source result before the public map boundary.
type Result struct {
	fields []Field
}

// NewResult constructs a source result from fields in source insertion order.
// It applies BaseResult's named-field normalization only for truthy values,
// matching the frozen Python source.
func NewResult(fields []Field) (Result, error) {
	result := Result{fields: make([]Field, 0, len(fields))}
	for _, field := range fields {
		if err := result.set(field); err != nil {
			return Result{}, err
		}
	}
	return result, nil
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

// Fields returns source fields in insertion order.
func (r Result) Fields() []Field {
	fields := make([]Field, len(r.fields))
	copy(fields, r.fields)
	return fields
}

// Map returns raw result values by field name. Result retains source ordering
// internally because Go map iteration cannot carry that compatibility contract.
func (r Result) Map() map[string]any {
	values := make(map[string]any, len(r.fields))
	for _, field := range r.fields {
		values[field.Name] = field.Value
	}
	return values
}

// Value returns one raw source value by field name.
func (r Result) Value(name string) (any, bool) {
	for _, field := range r.fields {
		if field.Name == name {
			return field.Value, true
		}
	}
	return nil, false
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
		return nil, fmt.Errorf("%w: %q", ErrUnknownResultCategory, category)
	}
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
