package engine

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/jcastillo/goddgs/internal/parser"
	"github.com/jcastillo/goddgs/internal/transport"
)

type jsonTextTransport interface {
	Do(context.Context, transport.Request) (transport.Response, error)
}

// Grokipedia adapts frozen Grokipedia JSON text behavior.
type Grokipedia struct {
	transport jsonTextTransport
}

var _ Searcher = (*Grokipedia)(nil)

// NewGrokipedia constructs a Grokipedia adapter.
func NewGrokipedia(client jsonTextTransport) *Grokipedia {
	return &Grokipedia{transport: client}
}

// Wikipedia adapts frozen Wikipedia JSON text behavior.
type Wikipedia struct {
	transport jsonTextTransport
}

var _ Searcher = (*Wikipedia)(nil)

// NewWikipedia constructs a Wikipedia adapter.
func NewWikipedia(client jsonTextTransport) *Wikipedia {
	return &Wikipedia{transport: client}
}

type sourceEngineError struct {
	sourceType string
	message    string
	cause      error
}

func (errorValue *sourceEngineError) Error() string {
	return errorValue.message
}

func (errorValue *sourceEngineError) Unwrap() error {
	return errorValue.cause
}

func newSourceEngineError(sourceType, message string, cause error) *sourceEngineError {
	return &sourceEngineError{sourceType: sourceType, message: message, cause: cause}
}

func sourceInteger(value int) string {
	return strconv.Itoa(value)
}

func sourceTypeName(value any) string {
	switch typed := value.(type) {
	case nil:
		return "NoneType"
	case bool:
		return "bool"
	case string:
		return "str"
	case json.Number:
		if strings.ContainsAny(typed.String(), ".eE") {
			return "float"
		}
		return "int"
	case float64:
		return "float"
	case []any:
		return "list"
	case map[string]any, *parser.OrderedObject:
		return "dict"
	default:
		return "object"
	}
}

func sourcePythonString(value any) string {
	switch value := value.(type) {
	case nil:
		return "None"
	case string:
		return value
	case bool:
		if value {
			return "True"
		}
		return "False"
	case json.Number:
		return sourcePythonNumberString(value)
	case float64:
		return sourcePythonFloatString(value)
	case []any:
		items := make([]string, len(value))
		for index, item := range value {
			items[index] = sourcePythonRepr(item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	case *parser.OrderedObject:
		fields := value.Fields()
		items := make([]string, len(fields))
		for index, field := range fields {
			items[index] = sourcePythonRepr(field.Name) + ": " + sourcePythonRepr(field.Value)
		}
		return "{" + strings.Join(items, ", ") + "}"
	default:
		return "<unsupported source string>"
	}
}

func sourcePythonFloatString(value float64) string {
	switch {
	case math.IsNaN(value):
		return "nan"
	case math.IsInf(value, 1):
		return "inf"
	case math.IsInf(value, -1):
		return "-inf"
	default:
		return strconv.FormatFloat(value, 'g', -1, 64)
	}
}

func sourcePythonNumberString(value json.Number) string {
	raw := value.String()
	if !strings.ContainsAny(raw, ".eE") {
		if raw == "-0" {
			return "0"
		}
		return raw
	}

	number, err := strconv.ParseFloat(raw, 64)
	if math.IsInf(number, 1) {
		return "inf"
	}
	if math.IsInf(number, -1) {
		return "-inf"
	}
	if err != nil {
		return raw
	}
	if number == 0 && math.Signbit(number) {
		return "-0.0"
	}

	scientific := strconv.FormatFloat(number, 'e', -1, 64)
	mantissa, exponentText, _ := strings.Cut(scientific, "e")
	exponent, err := strconv.Atoi(exponentText)
	if err != nil {
		return raw
	}
	if exponent >= -4 && exponent < 16 {
		decimal := sourcePythonDecimal(mantissa, exponent)
		if !strings.ContainsRune(decimal, '.') {
			return decimal + ".0"
		}
		return decimal
	}
	return mantissa + "e" + sourcePythonExponent(exponent)
}

func sourcePythonDecimal(mantissa string, exponent int) string {
	sign := ""
	if strings.HasPrefix(mantissa, "-") {
		sign = "-"
		mantissa = strings.TrimPrefix(mantissa, "-")
	}
	digits := strings.ReplaceAll(mantissa, ".", "")
	decimalPosition := 1 + exponent
	switch {
	case decimalPosition <= 0:
		return sign + "0." + strings.Repeat("0", -decimalPosition) + digits
	case decimalPosition >= len(digits):
		return sign + digits + strings.Repeat("0", decimalPosition-len(digits))
	default:
		return sign + digits[:decimalPosition] + "." + digits[decimalPosition:]
	}
}

func sourcePythonExponent(value int) string {
	sign := "+"
	if value < 0 {
		sign = "-"
		value = -value
	}
	digits := strconv.Itoa(value)
	if len(digits) == 1 {
		digits = "0" + digits
	}
	return sign + digits
}

func sourcePythonRepr(value any) string {
	if text, ok := value.(string); ok {
		return sourcePythonStringRepr(text)
	}
	return sourcePythonString(value)
}

func sourcePythonStringRepr(value string) string {
	quote := byte('\'')
	if strings.ContainsRune(value, '\'') && !strings.ContainsRune(value, '"') {
		quote = '"'
	}

	var builder strings.Builder
	builder.Grow(len(value) + 2)
	builder.WriteByte(quote)
	for _, character := range value {
		switch character {
		case '\\':
			builder.WriteString(`\\`)
		case '\t':
			builder.WriteString(`\t`)
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		default:
			if character == rune(quote) {
				builder.WriteByte('\\')
				builder.WriteRune(character)
				continue
			}
			if unicode.IsPrint(character) {
				builder.WriteRune(character)
				continue
			}
			sourcePythonEscape(&builder, character)
		}
	}
	builder.WriteByte(quote)
	return builder.String()
}

func sourcePythonEscape(builder *strings.Builder, value rune) {
	const hexadecimal = "0123456789abcdef"

	if value <= 0xff {
		builder.WriteString(`\x`)
		builder.WriteByte(hexadecimal[value>>4])
		builder.WriteByte(hexadecimal[value&0x0f])
		return
	}
	if value <= 0xffff {
		builder.WriteString(`\u`)
		for shift := uint(12); ; shift -= 4 {
			builder.WriteByte(hexadecimal[(value>>shift)&0x0f])
			if shift == 0 {
				return
			}
		}
	}
	builder.WriteString(`\U`)
	for shift := uint(28); ; shift -= 4 {
		builder.WriteByte(hexadecimal[(value>>shift)&0x0f])
		if shift == 0 {
			return
		}
	}
}
