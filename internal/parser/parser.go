package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/lestrrat-go/helium"
	"github.com/lestrrat-go/helium/html"
	"github.com/lestrrat-go/helium/xpath1"
)

var errTrailingJSONValue = errors.New("extra JSON value")

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
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errTrailingJSONValue
		}
		return nil, err
	}
	return value, nil
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
