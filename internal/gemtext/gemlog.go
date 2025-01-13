package gemtext

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/feeds"
)

// feedItemSeparators are different separator characters that someone might use
// to separate the date string from the link description in a gemlog.
var feedItemSeparators = "-:|"

// FeedTranslator is used to translate a gemtext file, interpreted as a
// [gemlog], into an RSS, Atom, or JSON feed.
//
// [gemlog]: https://geminiprotocol.net/docs/companion/subscription.gmi
type FeedTranslator struct {

	// Required. When interpreting links from the gemlog, all links will be
	// interpreted as being relative to this URL.
	BaseURL *url.URL

	// Optional strings to use in the top-level 'author' field of the resulting
	// feed.
	AuthorName, AuthorEmail string
}

func (t FeedTranslator) toFeed(src io.Reader) (*feeds.Feed, error) {
	var (
		r          = bufio.NewReader(src)
		baseURLStr = t.BaseURL.String()
		feed       = &feeds.Feed{
			Link: &feeds.Link{Href: baseURLStr},
			Id:   baseURLStr,
		}
	)

	if t.AuthorName != "" || t.AuthorEmail != "" {
		feed.Author = &feeds.Author{
			Name:  t.AuthorName,
			Email: t.AuthorEmail,
		}
	}

loop:
	for {
		line, err := r.ReadString('\n')

		switch {
		case errors.Is(err, io.EOF):
			break loop

		case err != nil:
			return nil, fmt.Errorf("reading next line: %w", err)

		case strings.HasPrefix(line, "#"):
			feed.Title = strings.TrimSpace(line[1:])

		case strings.HasPrefix(line, "=>"):
			parsedLink := parseLinkLine(line)

			if len(parsedLink.label) < 10 {
				continue
			}

			date, err := time.Parse("2006-01-02", parsedLink.label[:10])
			if err != nil {
				continue
			}

			// "An entry's required "updated" element is noon UTC on the day
			// indicated by the 10 character date stamp at the beginning of the
			// corresponding link line's label."
			updatedAt := time.Date(
				date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, time.UTC,
			)

			title := strings.TrimSpace(parsedLink.label[10:])
			for {
				prevTitle := title
				title = strings.TrimLeft(title, feedItemSeparators)
				title = strings.TrimSpace(title)
				if title == prevTitle {
					break
				}
			}

			url, err := url.Parse(parsedLink.url)
			if err != nil {
				continue
			}

			absURL := t.BaseURL.ResolveReference(url)

			feed.Items = append(feed.Items, &feeds.Item{
				Title:   title,
				Link:    &feeds.Link{Href: absURL.String(), Rel: "alternate"},
				Id:      absURL.String(),
				Updated: updatedAt,
			})

			if updatedAt.After(feed.Updated) {
				feed.Updated = updatedAt
			}
		}
	}

	if feed.Updated.IsZero() {
		// "If no entries can be extracted from the document ... the feed's
		// "updated" element should be set equal to the time the document was
		// fetched."
		feed.Updated = time.Now().UTC()
	}

	return feed, nil
}

func (t FeedTranslator) translate(
	out io.Writer, in io.Reader, fn func(*feeds.Feed) (string, error),
) error {
	feed, err := t.toFeed(in)
	if err != nil {
		return fmt.Errorf("translating document to feed: %w", err)
	}

	outStr, err := fn(feed)
	if err != nil {
		return fmt.Errorf("rendering feed: %w", err)
	}

	if _, err := out.Write([]byte(outStr)); err != nil {
		return fmt.Errorf("writing feed: %w", err)
	}

	return nil
}

// ToRSS translates the input gemtext document into an RSS feed.
func (t FeedTranslator) ToRSS(to io.Writer, from io.Reader) error {
	return t.translate(to, from, (*feeds.Feed).ToRss)
}

// ToAtom translates the input gemtext document into an Atom feed.
func (t FeedTranslator) ToAtom(to io.Writer, from io.Reader) error {
	return t.translate(to, from, (*feeds.Feed).ToAtom)
}

// ToJSON translates the input gemtext document into an JSON feed.
func (t FeedTranslator) ToJSON(to io.Writer, from io.Reader) error {
	return t.translate(to, from, (*feeds.Feed).ToJSON)
}
