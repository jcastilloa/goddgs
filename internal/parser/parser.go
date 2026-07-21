package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/html"
	"github.com/lestrrat-go/helium/xpath1"
)

var errTrailingJSONValue = errors.New("extra JSON value")

// JSONDecodeError classifies a source JSON decoding failure.
type JSONDecodeError struct {
	message string
	cause   error
}

func (errorValue *JSONDecodeError) Error() string {
	return errorValue.message
}

func (errorValue *JSONDecodeError) Unwrap() error {
	return errorValue.cause
}

type trailingJSONValueError struct {
	firstValueEnd int64
}

func (trailingJSONValueError) Error() string {
	return errTrailingJSONValue.Error()
}

// Document is an internal parsed HTML document. It intentionally hides the
// third-party DOM so engine adapters depend only on source-compatible XPath
// operations.
type Document struct {
	node helium.Node
}

// FieldQuery associates a source result field with its frozen XPath expression.
// Callers must pass queries in source declaration order.
type FieldQuery struct {
	Name  string
	XPath string
}

// Field contains raw XPath strings and the source-compatible joined form.
type Field struct {
	Name   string
	Raw    []string
	Joined string
}

// Item is one source item selected by an engine items XPath expression.
type Item struct {
	Fields []Field
}

// OrderedField is one JSON object member in source input order.
type OrderedField struct {
	Name  string
	Value any
}

// OrderedObject retains JSON object input order for source adapters that
// iterate object members.
type OrderedObject struct {
	fields []OrderedField
}

// ParseHTML parses one engine response into an internal document.
func ParseHTML(ctx context.Context, source string) (*Document, error) {
	node, err := html.NewParser().Parse(ctx, []byte(source))
	if err != nil {
		return nil, err
	}
	return &Document{node: node}, nil
}

// DecodeJSON decodes one source engine JSON response without narrowing values.
func DecodeJSON(source []byte) (any, error) {
	if hasPythonNonFiniteLiterals(source) {
		value, err := decodePythonJSON(source)
		if err != nil {
			return nil, err
		}
		return orderedJSONValueAsPlain(value), nil
	}

	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, newJSONDecodeError(source, err)
	}

	if err := rejectTrailingJSONValue(decoder, decoder.InputOffset()); err != nil {
		return nil, newJSONDecodeError(source, err)
	}
	return value, nil
}

// DecodeOrderedJSON decodes one JSON value while retaining object member
// order for source-compatible iteration.
func DecodeOrderedJSON(source []byte) (any, error) {
	if hasPythonNonFiniteLiterals(source) {
		return decodePythonJSON(source)
	}

	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()

	value, err := decodeOrderedJSONValue(decoder)
	if err != nil {
		return nil, newJSONDecodeError(source, err)
	}
	if err := rejectTrailingJSONValue(decoder, decoder.InputOffset()); err != nil {
		return nil, newJSONDecodeError(source, err)
	}
	return value, nil
}

// Fields returns object members in JSON input order.
func (object *OrderedObject) Fields() []OrderedField {
	fields := make([]OrderedField, len(object.fields))
	copy(fields, object.fields)
	return fields
}

// Value returns one object member by name.
func (object *OrderedObject) Value(name string) (any, bool) {
	for _, field := range object.fields {
		if field.Name == name {
			return field.Value, true
		}
	}
	return nil, false
}

// RemoveCommentDelimiters mirrors Anna's Archive source preprocessing.
func RemoveCommentDelimiters(source string) string {
	return strings.ReplaceAll(strings.ReplaceAll(source, "<!--", ""), "-->", "")
}

// Values evaluates one XPath expression against the document.
func (document *Document) Values(ctx context.Context, expression string) ([]string, error) {
	return values(ctx, document.node, expression)
}

// Extract evaluates item and field XPath expressions in caller-supplied order.
func (document *Document) Extract(ctx context.Context, itemsXPath string, queries []FieldQuery) ([]Item, error) {
	items, err := xpath1.Find(ctx, document.node, itemsXPath)
	if err != nil {
		return nil, err
	}

	result := make([]Item, 0, len(items))
	for _, item := range items {
		fields := make([]Field, 0, len(queries))
		for _, query := range queries {
			raw, err := values(ctx, item, query.XPath)
			if err != nil {
				return nil, err
			}
			fields = append(fields, Field{
				Name:   query.Name,
				Raw:    raw,
				Joined: joinSourceXPathValues(raw),
			})
		}
		result = append(result, Item{Fields: fields})
	}
	return result, nil
}

func values(ctx context.Context, node helium.Node, expression string) ([]string, error) {
	nodes, err := xpath1.Find(ctx, node, expression)
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, string(node.Content()))
	}
	return result, nil
}

func joinSourceXPathValues(values []string) string {
	return strings.Join(strings.Fields(strings.Join(values, "")), " ")
}

func rejectTrailingJSONValue(decoder *json.Decoder, firstValueEnd int64) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return trailingJSONValueError{firstValueEnd: firstValueEnd}
		}
		return err
	}
	return nil
}

func decodeOrderedJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}

	switch delimiter {
	case '{':
		return decodeOrderedJSONObject(decoder)
	case '[':
		return decodeOrderedJSONArray(decoder)
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

func decodeOrderedJSONObject(decoder *json.Decoder) (*OrderedObject, error) {
	object := &OrderedObject{}
	indexes := make(map[string]int)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("JSON object key = %T, want string", token)
		}
		value, err := decodeOrderedJSONValue(decoder)
		if err != nil {
			return nil, err
		}
		if index, exists := indexes[name]; exists {
			object.fields[index].Value = value
			continue
		}
		indexes[name] = len(object.fields)
		object.fields = append(object.fields, OrderedField{Name: name, Value: value})
	}
	if err := consumeJSONDelimiter(decoder, '}'); err != nil {
		return nil, err
	}
	return object, nil
}

func decodeOrderedJSONArray(decoder *json.Decoder) ([]any, error) {
	values := make([]any, 0)
	for decoder.More() {
		value, err := decodeOrderedJSONValue(decoder)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := consumeJSONDelimiter(decoder, ']'); err != nil {
		return nil, err
	}
	return values, nil
}

func consumeJSONDelimiter(decoder *json.Decoder, want json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != want {
		return fmt.Errorf("JSON delimiter = %q, want %q", token, want)
	}
	return nil
}

func decodePythonJSON(source []byte) (any, error) {
	validator := json.NewDecoder(bytes.NewReader(replacePythonNonFiniteLiterals(source)))
	validator.UseNumber()
	var ignored any
	if err := validator.Decode(&ignored); err != nil {
		return nil, newJSONDecodeError(source, err)
	}
	if err := rejectTrailingJSONValue(validator, validator.InputOffset()); err != nil {
		return nil, newJSONDecodeError(source, err)
	}

	decoder := pythonJSONDecoder{source: source}
	value, err := decoder.decodeValue()
	if err != nil {
		return nil, newJSONDecodeError(source, err)
	}
	decoder.skipWhitespace()
	if decoder.index != len(source) {
		return nil, newJSONDecodeError(source, trailingJSONValueError{firstValueEnd: int64(decoder.index)})
	}
	return value, nil
}

type pythonJSONDecoder struct {
	source []byte
	index  int
}

func (decoder *pythonJSONDecoder) decodeValue() (any, error) {
	decoder.skipWhitespace()
	if decoder.index >= len(decoder.source) {
		return nil, io.ErrUnexpectedEOF
	}

	switch decoder.source[decoder.index] {
	case '{':
		return decoder.decodeObject()
	case '[':
		return decoder.decodeArray()
	case '"':
		return decoder.decodeString()
	case 't':
		return decoder.decodeLiteral("true", true)
	case 'f':
		return decoder.decodeLiteral("false", false)
	case 'n':
		return decoder.decodeLiteral("null", nil)
	case 'N':
		return decoder.decodeLiteral("NaN", math.NaN())
	case 'I':
		return decoder.decodeLiteral("Infinity", math.Inf(1))
	case '-':
		if decoder.matches("-Infinity") {
			return decoder.decodeLiteral("-Infinity", math.Inf(-1))
		}
		return decoder.decodeNumber()
	default:
		return decoder.decodeNumber()
	}
}

func (decoder *pythonJSONDecoder) decodeObject() (*OrderedObject, error) {
	decoder.index++
	decoder.skipWhitespace()

	object := &OrderedObject{}
	indexes := make(map[string]int)
	if decoder.consume('}') {
		return object, nil
	}
	for {
		name, err := decoder.decodeString()
		if err != nil {
			return nil, err
		}
		decoder.skipWhitespace()
		if !decoder.consume(':') {
			return nil, fmt.Errorf("expected JSON object delimiter ':'")
		}
		value, err := decoder.decodeValue()
		if err != nil {
			return nil, err
		}
		if index, exists := indexes[name]; exists {
			object.fields[index].Value = value
		} else {
			indexes[name] = len(object.fields)
			object.fields = append(object.fields, OrderedField{Name: name, Value: value})
		}

		decoder.skipWhitespace()
		if decoder.consume('}') {
			return object, nil
		}
		if !decoder.consume(',') {
			return nil, fmt.Errorf("expected JSON object delimiter ','")
		}
		decoder.skipWhitespace()
	}
}

func (decoder *pythonJSONDecoder) decodeArray() ([]any, error) {
	decoder.index++
	decoder.skipWhitespace()

	values := make([]any, 0)
	if decoder.consume(']') {
		return values, nil
	}
	for {
		value, err := decoder.decodeValue()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
		decoder.skipWhitespace()
		if decoder.consume(']') {
			return values, nil
		}
		if !decoder.consume(',') {
			return nil, fmt.Errorf("expected JSON array delimiter ','")
		}
		decoder.skipWhitespace()
	}
}

func (decoder *pythonJSONDecoder) decodeString() (string, error) {
	if decoder.index >= len(decoder.source) || decoder.source[decoder.index] != '"' {
		return "", fmt.Errorf("expected JSON string")
	}

	start := decoder.index
	escaped := false
	for decoder.index++; decoder.index < len(decoder.source); decoder.index++ {
		value := decoder.source[decoder.index]
		if escaped {
			escaped = false
			continue
		}
		if value == '\\' {
			escaped = true
			continue
		}
		if value != '"' {
			continue
		}
		decoder.index++

		var result string
		if err := json.Unmarshal(decoder.source[start:decoder.index], &result); err != nil {
			return "", err
		}
		return result, nil
	}
	return "", io.ErrUnexpectedEOF
}

func (decoder *pythonJSONDecoder) decodeLiteral(literal string, value any) (any, error) {
	if !decoder.matches(literal) {
		return nil, fmt.Errorf("expected JSON literal %q", literal)
	}
	decoder.index += len(literal)
	return value, nil
}

func (decoder *pythonJSONDecoder) decodeNumber() (json.Number, error) {
	start := decoder.index
	for decoder.index < len(decoder.source) && !isJSONValueDelimiter(decoder.source[decoder.index]) {
		decoder.index++
	}
	if start == decoder.index {
		return "", fmt.Errorf("expected JSON number")
	}
	return json.Number(decoder.source[start:decoder.index]), nil
}

func (decoder *pythonJSONDecoder) matches(value string) bool {
	return len(decoder.source)-decoder.index >= len(value) && string(decoder.source[decoder.index:decoder.index+len(value)]) == value
}

func (decoder *pythonJSONDecoder) consume(value byte) bool {
	if decoder.index >= len(decoder.source) || decoder.source[decoder.index] != value {
		return false
	}
	decoder.index++
	return true
}

func (decoder *pythonJSONDecoder) skipWhitespace() {
	for decoder.index < len(decoder.source) && isJSONWhitespace(decoder.source[decoder.index]) {
		decoder.index++
	}
}

func orderedJSONValueAsPlain(value any) any {
	switch typed := value.(type) {
	case *OrderedObject:
		result := make(map[string]any, len(typed.fields))
		for _, field := range typed.fields {
			result[field.Name] = orderedJSONValueAsPlain(field.Value)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = orderedJSONValueAsPlain(item)
		}
		return result
	default:
		return value
	}
}

func hasPythonNonFiniteLiterals(source []byte) bool {
	inString := false
	escaped := false

	for index := 0; index < len(source); index++ {
		value := source[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if value == '\\' {
				escaped = true
				continue
			}
			if value == '"' {
				inString = false
			}
			continue
		}
		if value == '"' {
			inString = true
			continue
		}

		for _, literal := range []string{"-Infinity", "Infinity", "NaN"} {
			if !hasJSONLiteralBoundary(source, index, literal) {
				continue
			}
			return true
		}
	}
	return false
}

func replacePythonNonFiniteLiterals(source []byte) []byte {
	result := append([]byte(nil), source...)
	inString := false
	escaped := false
	for index := 0; index < len(source); index++ {
		value := source[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if value == '\\' {
				escaped = true
				continue
			}
			if value == '"' {
				inString = false
			}
			continue
		}
		if value == '"' {
			inString = true
			continue
		}
		for _, literal := range []string{"-Infinity", "Infinity", "NaN"} {
			if !hasJSONLiteralBoundary(source, index, literal) {
				continue
			}
			for offset := range literal {
				result[index+offset] = ' '
			}
			if literal[0] == '-' {
				result[index] = '-'
				result[index+1] = '0'
			} else {
				result[index] = '0'
			}
			index += len(literal) - 1
			break
		}
	}
	return result
}

func hasJSONLiteralBoundary(source []byte, index int, literal string) bool {
	end := index + len(literal)
	if end > len(source) || string(source[index:end]) != literal {
		return false
	}
	if index > 0 && !isJSONValueStartDelimiter(source[index-1]) {
		return false
	}
	return end == len(source) || isJSONValueDelimiter(source[end])
}

func isJSONValueStartDelimiter(value byte) bool {
	return isJSONWhitespace(value) || strings.ContainsRune("[,:", rune(value))
}

func isJSONValueDelimiter(value byte) bool {
	return isJSONWhitespace(value) || strings.ContainsRune(",]}", rune(value))
}

func isJSONWhitespace(value byte) bool {
	return strings.ContainsRune(" \t\r\n", rune(value))
}

func newJSONDecodeError(source []byte, cause error) error {
	message := cause.Error()
	var trailing trailingJSONValueError
	switch {
	case errors.As(cause, &trailing):
		message = pythonJSONErrorMessage("Extra data", source, firstNonWhitespaceOffset(source, trailing.firstValueEnd))
	case errors.Is(cause, io.EOF) || strings.Contains(cause.Error(), "unexpected EOF"):
		if expectation, ok := incompleteJSONExpectation(source); ok {
			message = pythonJSONErrorMessage(expectation, source, len(source))
		}
	}
	return &JSONDecodeError{message: message, cause: cause}
}

func incompleteJSONExpectation(source []byte) (string, bool) {
	last, container := lastJSONSignificantByte(source)
	switch last {
	case '{':
		return "Expecting property name enclosed in double quotes", true
	case '[', ':':
		return "Expecting value", true
	case ',':
		if container == '{' {
			return "Expecting property name enclosed in double quotes", true
		}
		if container == '[' {
			return "Expecting value", true
		}
	}
	return "", false
}

func lastJSONSignificantByte(source []byte) (byte, byte) {
	var stack []byte
	inString := false
	escaped := false
	last := byte(0)
	for _, value := range source {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if value == '\\' {
				escaped = true
				continue
			}
			if value == '"' {
				inString = false
			}
			continue
		}
		if value == '"' {
			inString = true
			last = value
			continue
		}
		if strings.ContainsRune(" \t\r\n", rune(value)) {
			continue
		}
		switch value {
		case '{', '[':
			stack = append(stack, value)
		case '}', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
		last = value
	}
	if len(stack) == 0 {
		return last, 0
	}
	return last, stack[len(stack)-1]
}

func firstNonWhitespaceOffset(source []byte, offset int64) int {
	start := int(offset)
	if start < 0 {
		start = 0
	}
	if start > len(source) {
		start = len(source)
	}
	for start < len(source) && strings.ContainsRune(" \t\r\n", rune(source[start])) {
		start++
	}
	return start
}

func pythonJSONErrorMessage(expectation string, source []byte, byteOffset int) string {
	line, column, character := pythonJSONPosition(source, byteOffset)
	return fmt.Sprintf("%s: line %d column %d (char %d)", expectation, line, column, character)
}

func pythonJSONPosition(source []byte, byteOffset int) (int, int, int) {
	if byteOffset < 0 {
		byteOffset = 0
	}
	if byteOffset > len(source) {
		byteOffset = len(source)
	}
	line := 1
	column := 1
	characters := 0
	for _, value := range string(source[:byteOffset]) {
		characters++
		if value == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}
	return line, column, characters
}
