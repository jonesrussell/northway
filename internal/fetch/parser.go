// Package fetch owns external feed transport and parsing; it never follows item links.
package fetch

// Selected behavior adapted from jonesrussell/north-cloud at
// 51b877de7dab311c981dcdb4d38dfdca9965aeb1, crawler/internal/feed/parser.go.
// See docs/migration.md for ownership, dependencies and behavior changes.
import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/jonesrussell/northway/internal/ingest"
	"github.com/mmcdole/gofeed"
)

// ItemURL preserves URL meaning; it does not force schemes or remove slashes.
// Fragments are not part of the fetched document identity. Item URLs are stored,
// never resolved or requested, and cannot become a source configuration.
func ItemURL(raw string) (string, error) {
	if len(raw) == 0 || len(raw) > 2048 || !utf8.ValidString(raw) || strings.ContainsAny(raw, "\x00\r\n\t ") {
		return "", ingest.ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || (u.Port() != "" && u.Port() != "443") {
		return "", ingest.ErrInvalid
	}
	u.Fragment = ""
	return u.String(), nil
}

func validText(s string, max int) bool {
	return strings.TrimSpace(s) != "" && len(s) <= max && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}

// screenXML bounds token/depth/entry work before the compatibility parser runs.
// DTDs/entities and alternate encodings are unsupported, even if benign.
func screenXML(ctx context.Context, body []byte) error {
	if len(body) == 0 || int64(len(body)) > ingest.MaxResponseBytes || !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return ingest.ErrParse
	}
	d := xml.NewDecoder(bytes.NewReader(body))
	depth, tokens, entries, roots := 0, 0, 0, 0
	for {
		if ctx.Err() != nil {
			return ingest.ErrParse
		}
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ingest.ErrParse
		}
		tokens++
		if tokens > 60000 {
			return ingest.ErrParse
		}
		switch v := tok.(type) {
		case xml.Directive:
			return ingest.ErrParse
		case xml.ProcInst:
			if v.Target != "xml" || roots != 0 {
				return ingest.ErrParse
			}
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || !((v.Name.Local == "rss") || (v.Name.Local == "feed" && v.Name.Space == "http://www.w3.org/2005/Atom")) {
					return ingest.ErrParse
				}
			}
			depth++
			if depth > 32 || len(v.Attr) > 128 {
				return ingest.ErrParse
			}
			if v.Name.Local == "item" || v.Name.Local == "entry" {
				entries++
				if entries > ingest.MaxItems {
					return ingest.ErrParse
				}
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(v)) != 0 {
				return ingest.ErrParse
			}
		}
	}
	if roots != 1 || depth != 0 {
		return ingest.ErrParse
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func Parse(ctx context.Context, body []byte) ([]ingest.Item, error) {
	if err := screenXML(ctx, body); err != nil {
		return nil, err
	}
	parser := gofeed.NewParser()
	parsed, err := parser.Parse(contextReader{ctx, bytes.NewReader(body)})
	if err != nil || len(parsed.Items) > ingest.MaxItems {
		return nil, ingest.ErrParse
	}
	items := make([]ingest.Item, 0, len(parsed.Items))
	seen := map[string]ingest.Item{}
	for _, entry := range parsed.Items {
		if ctx.Err() != nil {
			return nil, ingest.ErrParse
		}
		raw := entry.Link
		if raw == "" {
			raw = entry.GUID
		}
		link, err := ItemURL(raw)
		if err != nil || !validText(entry.Title, 512) {
			return nil, ingest.ErrParse
		}
		origin := entry.GUID
		if origin == "" {
			origin = link
		}
		if !validText(origin, 2048) {
			return nil, ingest.ErrParse
		}
		item := ingest.Item{OriginID: origin, URL: link, Title: entry.Title, PublishedAt: entry.PublishedParsed}
		if item.PublishedAt != nil {
			t := item.PublishedAt.UTC()
			if t.Year() < 1970 || t.Year() > 9999 {
				return nil, ingest.ErrParse
			}
			item.PublishedAt = &t
		}
		if old, ok := seen[origin]; ok {
			sameDate := old.PublishedAt == nil && item.PublishedAt == nil || old.PublishedAt != nil && item.PublishedAt != nil && old.PublishedAt.Equal(*item.PublishedAt)
			if old.URL != item.URL || old.Title != item.Title || !sameDate {
				return nil, ingest.ErrParse
			}
			continue
		}
		seen[origin] = item
		items = append(items, item)
	}
	return items, nil
}
