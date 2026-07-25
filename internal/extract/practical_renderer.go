package extract

import (
	"context"
	"strings"
	"unicode"

	markdown "github.com/JohannesKaufmann/html-to-markdown"
	"golang.org/x/net/html"
)

func (practicalRenderer) Render(ctx context.Context, source string) (Rendered, error) {
	if err := ctx.Err(); err != nil {
		return Rendered{}, err
	}

	converter := markdown.NewConverter("", true, &markdown.Options{
		HeadingStyle:     "atx",
		BulletListMarker: "*",
		LinkStyle:        "inlined",
	})
	converted, err := converter.ConvertString(source)
	if err != nil {
		return Rendered{}, err
	}
	if err := ctx.Err(); err != nil {
		return Rendered{}, err
	}

	return Rendered{
		Markdown: converted,
		Plain:    practicalPlainText(source),
		Rich:     converted,
	}, nil
}

func practicalPlainText(source string) string {
	document, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return ""
	}

	var output strings.Builder
	writePlainText(&output, document)
	return strings.TrimSpace(collapsePlainText(output.String()))
}

func writePlainText(output *strings.Builder, node *html.Node) {
	if node.Type == html.TextNode {
		output.WriteString(node.Data)
		return
	}
	if node.Type == html.ElementNode && plainTextIgnoredElement(node.Data) {
		return
	}

	if node.Type == html.ElementNode && plainTextBlockElement(node.Data) {
		writePlainBreak(output, plainTextBlockBreak(node.Data))
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		writePlainText(output, child)
	}
	if node.Type == html.ElementNode && plainTextBlockElement(node.Data) {
		writePlainBreak(output, plainTextBlockBreak(node.Data))
	}
}

func plainTextIgnoredElement(name string) bool {
	return name == "script" || name == "style" || name == "textarea"
}

func plainTextBlockElement(name string) bool {
	switch name {
	case "article", "blockquote", "div", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "li", "main", "ol", "p", "section", "ul":
		return true
	default:
		return false
	}
}

func plainTextBlockBreak(name string) string {
	if name == "li" {
		return "\n"
	}
	return "\n\n"
}

func writePlainBreak(output *strings.Builder, separator string) {
	if output.Len() > 0 && !strings.HasSuffix(output.String(), separator) {
		output.WriteString(separator)
	}
}

func collapsePlainText(source string) string {
	var output strings.Builder
	lineStart := true
	space := false
	newlines := 0
	for _, runeValue := range source {
		if runeValue == '\n' {
			if output.Len() == 0 {
				continue
			}
			newlines++
			space = false
			if newlines <= 2 {
				output.WriteRune('\n')
			}
			lineStart = true
			continue
		}
		if unicode.IsSpace(runeValue) {
			if !lineStart {
				space = true
			}
			continue
		}
		if space {
			output.WriteByte(' ')
		}
		output.WriteRune(runeValue)
		lineStart = false
		space = false
		newlines = 0
	}
	return output.String()
}
