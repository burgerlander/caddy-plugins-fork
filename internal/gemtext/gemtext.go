// Package gemtext implements shared logic related to gemtext files.
package gemtext

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"strings"
)

// HTMLTranslator is used to translate a gemtext file into equivalent HTML DOM
// elements.
type HTMLTranslator struct {
	// RenderHeading, if given can be used to override how headings are
	// rendered. The level indicates which heading level is being rendered: 1,
	// 2, or 3.
	RenderHeading func(w io.Writer, level int, text string) error

	// RenderLink, if given, can be used to override how links are rendered.
	RenderLink func(w io.Writer, url, label string) error
}

// HTML contains the result of a translation from gemtext. The Body will be the
// translated body itself, and Title will correspond to the first primary header
// of the gemtext file, if there was one.
type HTML struct {
	Title string
	Body  string
}

// Translate will read a gemtext file from the Reader and return it as an HTML
// document.
func (t HTMLTranslator) Translate(src io.Reader) (HTML, error) {
	var (
		r         = bufio.NewReader(src)
		w         = new(bytes.Buffer)
		title     string
		pft, list bool
		writeErr  error
	)

	sanitizeText := func(str string) string {
		return html.EscapeString(strings.TrimSpace(str))
	}

	write := func(fmtStr string, args ...any) {
		if writeErr != nil {
			return
		}
		_, writeErr = fmt.Fprintf(w, fmtStr, args...)
	}

loop:
	for {
		if writeErr != nil {
			return HTML{}, fmt.Errorf("writing line: %w", writeErr)
		}

		line, err := r.ReadString('\n')

		switch {
		case errors.Is(err, io.EOF):
			break loop

		case err != nil:
			return HTML{}, fmt.Errorf("reading next line: %w", err)

		case strings.HasPrefix(line, "```"):
			if !pft {
				write("<pre>\n")
				pft = true
			} else {
				write("</pre>\n")
				pft = false
			}
			continue

		case pft:
			write(html.EscapeString(line))
			continue

		case len(strings.TrimSpace(line)) == 0:
			continue
		}

		// list case is special, because it requires a prefix and suffix tag
		if strings.HasPrefix(line, "*") {
			if !list {
				write("<ul>\n")
			}
			write("<li>%s</li>\n", sanitizeText(line[1:]))
			list = true
			continue
		} else if list {
			write("</ul>\n")
			list = false
		}

		switch {
		case strings.HasPrefix(line, "=>"):
			var (
				parsedLink = parseLinkLine(line)
				urlStr     = parsedLink.url
				label      = sanitizeText(parsedLink.label)
			)

			if t.RenderLink == nil {
				write("<p><a href=\"%s\">%s</a></p>\n", urlStr, label)
			} else {
				writeErr = t.RenderLink(w, urlStr, label)
			}

		case strings.HasPrefix(line, "###"):
			text := sanitizeText(line[3:])
			if t.RenderHeading == nil {
				write("<h3>%s</h3>\n", text)
			} else {
				writeErr = t.RenderHeading(w, 3, text)
			}

		case strings.HasPrefix(line, "##"):
			text := sanitizeText(line[2:])
			if t.RenderHeading == nil {
				write("<h2>%s</h2>\n", text)
			} else {
				writeErr = t.RenderHeading(w, 2, text)
			}

		case strings.HasPrefix(line, "#"):
			text := sanitizeText(line[1:])
			if title == "" {
				title = text
			}

			if t.RenderHeading == nil {
				write("<h1>%s</h1>\n", text)
			} else {
				writeErr = t.RenderHeading(w, 1, text)
			}

		case strings.HasPrefix(line, ">"):
			write("<blockquote>%s</blockquote>\n", sanitizeText(line[1:]))

		default:
			line = strings.TrimSpace(line)
			write("<p>%s</p>\n", line)
		}
	}

	return HTML{
		Title: title,
		Body:  w.String(),
	}, nil
}
