package normalize

import (
	"bytes"
	"errors"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	utf8encoding "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const torBrowserProxy = "socks5h://127.0.0.1:9150"

const sourceDateLayout = "2006-01-02T15:04:05-07:00"

const (
	sourceMinimumTMYear int64 = -2_147_483_648
	sourceMaximumTMYear int64 = 2_147_483_647
	sourceTMYearOffset  int32 = 1900
)

var (
	stripTags          = regexp.MustCompile(`<.*?>`)
	pythonHTMLEntities = strings.NewReplacer(
		"&nGt;", "≫⃒",
		"&nLt;", "≪⃒",
	)
)

// ErrVQD classifies missing source VQD tokens.
var ErrVQD = errors.New("vqd token unavailable")

// ErrDate classifies source date-normalization failures.
var ErrDate = errors.New("date normalization failed")

// Text normalizes source text values.
func Text(raw string) string {
	if raw == "" {
		return ""
	}

	text := unescapeHTML(stripTags.ReplaceAllString(raw, ""))
	text = norm.NFC.String(text)
	text = removeUnicodeCategoryC(text)
	return strings.Join(strings.Fields(text), " ")
}

// URL normalizes source URL values.
func URL(value string) string {
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "%") {
		return strings.ReplaceAll(value, " ", "+")
	}

	decoded, _, _ := transform.String(utf8encoding.UTF8.NewDecoder(), percentDecode(value))
	return strings.ReplaceAll(decoded, " ", "+")
}

// Date normalizes source date values.
func Date(value any) (any, error) {
	seconds, ok := sourceInteger(value)
	if !ok {
		return value, nil
	}

	date := time.Unix(seconds, 0).UTC()
	year, err := sourceDateYear(date.Year())
	if err != nil {
		return nil, err
	}
	if year < 1 || year > 9999 {
		return nil, newDateError("ValueError", "year "+strconv.FormatInt(year, 10)+" is out of range")
	}
	return date.Format(sourceDateLayout), nil
}

// VQD extracts a DuckDuckGo VQD token from source response bytes.
func VQD(htmlBytes []byte, query string) (string, error) {
	for _, marker := range vqdMarkers {
		start := bytes.Index(htmlBytes, marker.prefix)
		if start < 0 {
			continue
		}
		start += len(marker.prefix)

		end := bytes.Index(htmlBytes[start:], marker.suffix)
		if end < 0 {
			continue
		}
		token := htmlBytes[start : start+end]
		if !utf8.Valid(token) {
			continue
		}
		return string(token), nil
	}
	return "", vqdError{query: query}
}

// Proxy expands a source proxy alias.
func Proxy(proxy *string) *string {
	if proxy == nil || *proxy != "tb" {
		return proxy
	}
	expanded := torBrowserProxy
	return &expanded
}

type vqdMarker struct {
	prefix []byte
	suffix []byte
}

var vqdMarkers = [...]vqdMarker{
	{prefix: []byte(`vqd="`), suffix: []byte(`"`)},
	{prefix: []byte("vqd="), suffix: []byte("&")},
	{prefix: []byte("vqd='"), suffix: []byte("'")},
}

type vqdError struct {
	query string
}

type dateError struct {
	sourceType string
	message    string
}

func (e *dateError) Error() string {
	return e.message
}

func (*dateError) Is(target error) bool {
	return target == ErrDate
}

func (e vqdError) Error() string {
	return "_extract_vqd() query=" + pythonStringRepr(e.query) + " Could not extract vqd."
}

func (vqdError) Is(target error) bool {
	return target == ErrVQD
}

func removeUnicodeCategoryC(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if unicode.Is(unicode.C, r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func unescapeHTML(value string) string {
	return html.UnescapeString(pythonHTMLEntities.Replace(value))
}

func percentDecode(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for index := 0; index < len(value); {
		if value[index] == '%' && index+2 < len(value) && isHex(value[index+1]) && isHex(value[index+2]) {
			builder.WriteByte(unhex(value[index+1])<<4 | unhex(value[index+2]))
			index += 3
			continue
		}
		builder.WriteByte(value[index])
		index++
	}
	return builder.String()
}

func isHex(value byte) bool {
	return '0' <= value && value <= '9' || 'a' <= value && value <= 'f' || 'A' <= value && value <= 'F'
}

func unhex(value byte) byte {
	if '0' <= value && value <= '9' {
		return value - '0'
	}
	if 'a' <= value && value <= 'f' {
		return value - 'a' + 10
	}
	return value - 'A' + 10
}

func sourceInteger(value any) (int64, bool) {
	switch value := value.(type) {
	case bool:
		if value {
			return 1, true
		}
		return 0, true
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	default:
		return 0, false
	}
}

func sourceDateYear(year int) (int64, error) {
	tmYear := int64(year) - int64(sourceTMYearOffset)
	if tmYear < sourceMinimumTMYear || tmYear > sourceMaximumTMYear {
		return 0, newDateError("OSError", "[Errno 75] Value too large for defined data type")
	}
	// Frozen CPython/Linux stores tm_year in signed int32 before adding 1900.
	return int64(int32(tmYear) + sourceTMYearOffset), nil
}

func newDateError(sourceType, message string) *dateError {
	return &dateError{sourceType: sourceType, message: message}
}

func pythonStringRepr(value string) string {
	quote := byte('\'')
	if strings.ContainsRune(value, '\'') && !strings.ContainsRune(value, '"') {
		quote = '"'
	}

	var builder strings.Builder
	builder.Grow(len(value) + 2)
	builder.WriteByte(quote)
	for _, r := range value {
		switch r {
		case '\\':
			builder.WriteString(`\\`)
		case '\t':
			builder.WriteString(`\t`)
		case '\n':
			builder.WriteString(`\n`)
		case '\r':
			builder.WriteString(`\r`)
		default:
			if r == rune(quote) {
				builder.WriteByte('\\')
				builder.WriteRune(r)
				continue
			}
			if unicode.IsPrint(r) {
				builder.WriteRune(r)
				continue
			}
			appendPythonEscape(&builder, r)
		}
	}
	builder.WriteByte(quote)
	return builder.String()
}

func appendPythonEscape(builder *strings.Builder, value rune) {
	const hexDigits = "0123456789abcdef"
	if value <= 0xff {
		builder.WriteString(`\x`)
		builder.WriteByte(hexDigits[value>>4])
		builder.WriteByte(hexDigits[value&0x0f])
		return
	}
	if value <= 0xffff {
		builder.WriteString(`\u`)
		for shift := uint(12); ; shift -= 4 {
			builder.WriteByte(hexDigits[(value>>shift)&0x0f])
			if shift == 0 {
				return
			}
		}
	}
	builder.WriteString(`\U`)
	for shift := uint(28); ; shift -= 4 {
		builder.WriteByte(hexDigits[(value>>shift)&0x0f])
		if shift == 0 {
			return
		}
	}
}
