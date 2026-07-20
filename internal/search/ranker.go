package search

import (
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const defaultRankerMinTokenLength = 3

// SimpleFilterRanker preserves the frozen source's deterministic result
// bucketing behavior before the scheduler applies its final result limit.
type SimpleFilterRanker struct {
	minTokenLength int
}

// NewSimpleFilterRanker constructs the frozen source's default ranker.
func NewSimpleFilterRanker() *SimpleFilterRanker {
	return &SimpleFilterRanker{minTokenLength: defaultRankerMinTokenLength}
}

// Tokens returns the source query tokens in deterministic order for fixture
// comparison. Ranking itself needs only membership, not token order.
func (r *SimpleFilterRanker) Tokens(query string) []string {
	tokens := r.extractTokens(query)
	result := make([]string, 0, len(tokens))
	for token := range tokens {
		result = append(result, token)
	}
	sort.Strings(result)
	return result
}

// Rank returns source documents in the frozen Wikipedia and token-match
// bucket order. It does not copy or normalize the raw source maps.
func (r *SimpleFilterRanker) Rank(documents []map[string]any, query string) ([]map[string]any, error) {
	tokens := r.extractTokens(query)

	var wikipedia, both, titleOnly, bodyOnly, neither []map[string]any
	for _, document := range documents {
		href, title, body, err := rankerDocumentValues(document)
		if err != nil {
			return nil, err
		}

		category, err := sourceContains(title, "Category:")
		if err != nil {
			return nil, err
		}
		if category {
			wikimedia, err := sourceContains(title, "Wikimedia")
			if err != nil {
				return nil, err
			}
			if wikimedia {
				continue
			}
		}

		wiki, err := sourceContains(href, "wikipedia.org")
		if err != nil {
			return nil, err
		}
		if wiki {
			wikipedia = append(wikipedia, document)
			continue
		}

		hitTitle, err := r.hasAnyToken(title, tokens)
		if err != nil {
			return nil, err
		}
		hitBody, err := r.hasAnyToken(body, tokens)
		if err != nil {
			return nil, err
		}

		switch {
		case hitTitle && hitBody:
			both = append(both, document)
		case hitTitle:
			titleOnly = append(titleOnly, document)
		case hitBody:
			bodyOnly = append(bodyOnly, document)
		default:
			neither = append(neither, document)
		}
	}

	ranked := make([]map[string]any, 0, len(documents))
	ranked = append(ranked, wikipedia...)
	ranked = append(ranked, both...)
	ranked = append(ranked, titleOnly...)
	ranked = append(ranked, bodyOnly...)
	ranked = append(ranked, neither...)
	return ranked, nil
}

func (r *SimpleFilterRanker) extractTokens(query string) map[string]struct{} {
	tokens := make(map[string]struct{})
	var token strings.Builder
	for _, character := range sourceLower(query) {
		if sourceWordCharacter(character) {
			token.WriteRune(character)
			continue
		}
		r.addToken(tokens, token.String())
		token.Reset()
	}
	r.addToken(tokens, token.String())
	return tokens
}

func (r *SimpleFilterRanker) addToken(tokens map[string]struct{}, token string) {
	if utf8.RuneCountInString(token) >= r.minTokenLength {
		tokens[token] = struct{}{}
	}
}

func (r *SimpleFilterRanker) hasAnyToken(value any, tokens map[string]struct{}) (bool, error) {
	text, err := sourceLowerValue(value)
	if err != nil {
		return false, err
	}
	for token := range tokens {
		if strings.Contains(text, token) {
			return true, nil
		}
	}
	return false, nil
}

func rankerDocumentValues(document map[string]any) (href, title, body any, err error) {
	if document == nil {
		return nil, nil, nil, errors.New("'NoneType' object has no attribute 'get'")
	}

	href = documentValue(document, "href")
	title = documentValue(document, "title")
	body, exists := document["body"]
	if !exists {
		body = documentValue(document, "description")
	}
	return href, title, body, nil
}

func documentValue(document map[string]any, name string) any {
	value, exists := document[name]
	if !exists {
		return ""
	}
	return value
}

func sourceContains(value any, needle string) (bool, error) {
	switch value := value.(type) {
	case string:
		return strings.Contains(value, needle), nil
	case []any:
		for _, item := range value {
			if item == needle {
				return true, nil
			}
		}
		return false, nil
	case map[string]any:
		_, exists := value[needle]
		return exists, nil
	case nil:
		return false, sourceNotIterableError("NoneType")
	default:
		return false, sourceNotIterableError(sourceTypeName(value))
	}
}

func sourceLowerValue(value any) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", errors.New("'" + sourceTypeName(value) + "' object has no attribute 'lower'")
	}
	return sourceLower(text), nil
}

func sourceLower(text string) string {
	return cases.Lower(language.Und).String(text)
}

func sourceWordCharacter(character rune) bool {
	return character == '_' || unicode.IsLetter(character) || unicode.IsNumber(character)
}

func sourceNotIterableError(typeName string) error {
	return errors.New("argument of type '" + typeName + "' is not iterable")
}
