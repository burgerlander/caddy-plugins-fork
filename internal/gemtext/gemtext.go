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
		fmt.Fprintf(w, fmtStr, args...)
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
				line   = strings.TrimSpace(line[2:])
				urlStr = line
				label  = urlStr
			)

			if i := strings.IndexAny(urlStr, " \t"); i > -1 {
				urlStr, label = urlStr[:i], sanitizeText(urlStr[i:])
			}

			if t.RenderLink == nil {
				write("<p><a href=\"%s\">%s</a></p>\n", urlStr, label)
			} else {
				if err := t.RenderLink(w, urlStr, label); err != nil {
					return HTML{}, fmt.Errorf(
						"rendering link %q (label:%q): %w", urlStr, label, err,
					)
				}
			}

		case strings.HasPrefix(line, "###"):
			write("<h3>%s</h3>\n", sanitizeText(line[3:]))

		case strings.HasPrefix(line, "##"):
			write("<h2>%s</h2>\n", sanitizeText(line[2:]))

		case strings.HasPrefix(line, "#"):
			line = sanitizeText(line[1:])
			if title == "" {
				title = line
			}
			write("<h1>%s</h1>\n", line)

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
