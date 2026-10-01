package fetch

// Selective metadata extraction concept adapted from north-cloud
// crawler/internal/fetcher/extractor.go at 51b877de7dab311c981dcdb4d38dfdca9965aeb1.
// No Colly, article body extraction, crawler visits or media downloads.
import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jonesrussell/northway/internal/ingest"
	"golang.org/x/net/html"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func (c *Client) FetchCollection(ctx context.Context, cl ingest.Claim) ingest.Result {
	if cl.Mode != "html" || !cl.RobotsUntil.After(time.Now().UTC()) {
		return ingest.Result{Failure: "transport"}
	}
	r, data := c.fetchDocument(ctx, cl, "text/html")
	if r.Status == 404 || r.Status == 410 {
		r.Failure = ""
		return r
	}
	if r.Failure == "" && r.Status == 200 {
		items, e := ExtractCreator(ctx, cl.URL, data, cl.PreviewAllowed)
		if e != nil {
			r.Failure = "parse"
		} else {
			r.Observations = items
		}
	}
	return r
}
func clean(s string, n int) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
	if len(s) > n {
		s = s[:n]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}
func reference(base, raw string) string {
	b, e := url.Parse(base)
	if e != nil {
		return ""
	}
	u, e := url.Parse(raw)
	if e != nil {
		return ""
	}
	v, e := ItemURL(b.ResolveReference(u).String())
	if e != nil {
		return ""
	}
	return v
}

// ExtractCreator parses approved bytes. Metadata cannot authorize a URL fetch,
// a creator identity merge, preview rights, publication or schedule.
func ExtractCreator(ctx context.Context, page string, body []byte, preview bool) ([]ingest.Observation, error) {
	if _, e := SourceURL(page); e != nil || len(body) == 0 || int64(len(body)) >= ingest.MaxResponseBytes || !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return nil, ingest.ErrParse
	}
	title, description, image := "", "", ""
	var schemas []map[string]any
	var links []string
	z := html.NewTokenizer(bytes.NewReader(body))
	tokens := 0
	script := false
	var scriptText strings.Builder
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		tt := z.Next()
		tokens++
		if tokens > 60000 {
			return nil, ingest.ErrParse
		}
		if tt == html.ErrorToken {
			if z.Err() == io.EOF {
				break
			}
			return nil, ingest.ErrParse
		}
		t := z.Token()
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			attrs := map[string]string{}
			if len(t.Attr) > 128 {
				return nil, ingest.ErrParse
			}
			for _, a := range t.Attr {
				attrs[a.Key] = a.Val
			}
			if t.Data == "meta" {
				switch strings.ToLower(attrs["property"]) {
				case "og:title":
					title = clean(attrs["content"], 512)
				case "og:description":
					description = clean(attrs["content"], 2048)
				case "og:image":
					image = reference(page, attrs["content"])
				}
			}
			if t.Data == "a" {
				for _, rel := range strings.Fields(attrs["rel"]) {
					if rel == "me" {
						if v := reference(page, attrs["href"]); v != "" {
							links = append(links, v)
						}
					}
				}
			}
			if t.Data == "script" && strings.EqualFold(attrs["type"], "application/ld+json") {
				script = true
				scriptText.Reset()
			}
		}
		if tt == html.TextToken && script {
			if scriptText.Len()+len(t.Data) > 65536 {
				return nil, ingest.ErrParse
			}
			scriptText.WriteString(t.Data)
		}
		if tt == html.EndTagToken && t.Data == "script" && script {
			script = false
			var v any
			if json.Unmarshal([]byte(scriptText.String()), &v) == nil {
				nodes := 0
				var walk func(any, int) error
				walk = func(v any, depth int) error {
					nodes++
					if nodes > 256 || depth > 16 {
						return ingest.ErrParse
					}
					switch n := v.(type) {
					case map[string]any:
						schemas = append(schemas, n)
						for _, x := range n {
							if e := walk(x, depth+1); e != nil {
								return e
							}
						}
					case []any:
						for _, x := range n {
							if e := walk(x, depth+1); e != nil {
								return e
							}
						}
					}
					return nil
				}
				if e := walk(v, 0); e != nil {
					return nil, e
				}
			}
		}
	}
	value := func(m map[string]any, k string) string { s, _ := m[k].(string); return s }
	for _, m := range schemas {
		if value(m, "@type") == "ProfilePage" || value(m, "@type") == "Person" {
			if s := value(m, "name"); s != "" {
				title = clean(s, 512)
			}
			if s := value(m, "description"); s != "" {
				description = clean(s, 2048)
			}
			if image == "" {
				image = reference(page, value(m, "image"))
			}
		}
	}
	if title == "" {
		return nil, ingest.ErrParse
	}
	makeItem := func(kind, original, name string) ingest.Observation {
		return ingest.Observation{Version: 1, AccountURL: page, OriginalURL: original, EvidenceURL: page, Kind: kind, Title: clean(name, 512), Display: "link", State: "available", Method: "html-metadata-v1"}
	}
	profile := makeItem("profile", page, title)
	profile.Description = description
	out := []ingest.Observation{profile}
	seen := map[string]bool{"profile\x00" + page: true}
	add := func(o ingest.Observation) error {
		key := o.Kind + "\x00" + o.OriginalURL
		if seen[key] {
			return nil
		}
		if len(out) >= 100 {
			return ingest.ErrParse
		}
		seen[key] = true
		out = append(out, o)
		return nil
	}
	if preview && image != "" {
		o := makeItem("image", image, title)
		o.PreviewURL = image
		o.Display = "image"
		if e := add(o); e != nil {
			return nil, e
		}
	}
	for _, m := range schemas {
		if value(m, "@type") == "VideoObject" {
			original := reference(page, value(m, "url"))
			if original == "" {
				original = reference(page, value(m, "embedUrl"))
			}
			if original == "" {
				continue
			}
			name := value(m, "name")
			if name == "" {
				name = title
			}
			o := makeItem("video", original, name)
			o.Description = clean(value(m, "description"), 2048)
			embed := reference(page, value(m, "embedUrl"))
			if ingest.SafeEmbed(embed) {
				o.EmbedURL = embed
				o.Display = "embed"
			}
			if preview {
				o.PreviewURL = reference(page, value(m, "thumbnailUrl"))
			}
			if e := add(o); e != nil {
				return nil, e
			}
		}
		switch v := m["sameAs"].(type) {
		case string:
			links = append(links, v)
		case []any:
			for _, x := range v {
				if s, ok := x.(string); ok {
					links = append(links, s)
				}
			}
		}
	}
	for _, v := range links {
		if original := reference(page, v); original != "" && original != page {
			if e := add(makeItem("link", original, title+" profile")); e != nil {
				return nil, e
			}
		}
	}
	return out, nil
}
