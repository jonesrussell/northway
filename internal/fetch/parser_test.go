package fetch

// RSS/Atom, missing-link and GUID fallback scenarios adapted from North Cloud
// crawler/internal/feed/parser_test.go at 51b877de7dab311c981dcdb4d38dfdca9965aeb1.
// All bodies below are newly authored synthetic fixtures.
import (
	"context"
	"fmt"
	"strings"
	"testing"
)

const rss = `<?xml version="1.0"?><rss version="2.0"><channel><title>Synthetic</title><link>https://example.invalid/</link><description>Fixture</description><item><guid>item-1</guid><title>Community programme</title><link>https://example.invalid/story/</link><description>Do not retain or follow https://127.0.0.1/secret</description><enclosure url="https://example.invalid/image" type="image/png" length="1"/></item></channel></rss>`
const atom = `<feed xmlns="http://www.w3.org/2005/Atom"><title>Synthetic</title><entry><id>release-1</id><title>Development release</title><link rel="alternate" href="https://example.invalid/release"/><published>2026-08-30T12:00:00Z</published></entry></feed>`

func TestParseMetadataOnly(t *testing.T) {
	for _, input := range []string{rss, atom} {
		items, err := Parse(t.Context(), []byte(input))
		if err != nil || len(items) != 1 {
			t.Fatalf("%v %v", items, err)
		}
		if items[0].OriginID == "" || items[0].Title == "" {
			t.Fatal(items)
		}
	}
	items, err := Parse(t.Context(), []byte(rss))
	if err != nil {
		t.Fatal(err)
	}
	if items[0].PublishedAt != nil || items[0].URL != "https://example.invalid/story/" {
		t.Fatal("changed unknown date or URL", items)
	}
	fallback := strings.Replace(rss, "<guid>item-1</guid>", "<guid>https://example.invalid/id</guid>", 1)
	fallback = strings.Replace(fallback, "<link>https://example.invalid/story/</link>", "", 1)
	items, err = Parse(t.Context(), []byte(fallback))
	if err != nil || items[0].URL != "https://example.invalid/id" {
		t.Fatal(items, err)
	}
}

func TestParserRejectsUnsafeOrUnboundedInputs(t *testing.T) {
	cases := []string{
		"", "<html>not a feed</html>", `{"version":"https://jsonfeed.org/version/1"}`,
		strings.Replace(rss, "<rss", `<!DOCTYPE rss [<!ENTITY external SYSTEM "file:///etc/passwd">]><rss`, 1),
		rss + rss, strings.Replace(rss, "<title>Community programme</title>", "<title>"+strings.Repeat("x", 513)+"</title>", 1),
		strings.Replace(rss, "https://example.invalid/story/", "http://example.invalid/story/", 1),
		strings.Replace(rss, "https://example.invalid/story/", "https://user:pass@example.invalid/", 1),
		strings.Replace(rss, "<item>", strings.Repeat("<nested>", 33)+"<item>", 1),
		"<rss><channel>" + strings.Repeat("<item/>", 1001) + "</channel></rss>",
		strings.Replace(rss, "<guid>item-1</guid>", "<guid>"+strings.Repeat("x", 2049)+"</guid>", 1),
		strings.Replace(rss, "<title>Synthetic</title>", "<title>\x00</title>", 1),
	}
	for i, input := range cases {
		if _, err := Parse(t.Context(), []byte(input)); err == nil {
			t.Fatalf("accepted case %d", i)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Parse(ctx, []byte(rss)); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestDuplicateIdentity(t *testing.T) {
	start := strings.Index(rss, "<item>")
	end := strings.Index(rss, "</item>") + len("</item>")
	item := rss[start:end]
	duplicate := strings.Replace(rss, "</channel>", item+"</channel>", 1)
	items, err := Parse(t.Context(), []byte(duplicate))
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	conflict := strings.Replace(rss, "</channel>", strings.Replace(item, "Community programme", "Conflicting title", 1)+"</channel>", 1)
	if _, err := Parse(t.Context(), []byte(conflict)); err == nil {
		t.Fatal("ambiguous identity accepted")
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte(rss))
	f.Add([]byte(atom))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 3*1024*1024 {
			return
		}
		Parse(t.Context(), data)
	})
}

func BenchmarkParseLargeFeed(b *testing.B) {
	var builder strings.Builder
	builder.WriteString(`<rss version="2.0"><channel><title>Synthetic</title>`)
	for i := 0; i < 870; i++ {
		fmt.Fprintf(&builder, `<item><guid>item-%d</guid><title>Development fixture %d</title><link>https://example.invalid/%d</link><description>%s</description></item>`, i, i, i, strings.Repeat("Synthetic discarded content. ", 50))
	}
	builder.WriteString("</channel></rss>")
	data := []byte(builder.String())
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		items, err := Parse(b.Context(), data)
		if err != nil || len(items) != 870 {
			b.Fatal(err, len(items))
		}
	}
}

func TestStylesheetPIIsInertAndTokenCapIndependent(t *testing.T) {
	withStyle := strings.Replace(rss, "?><rss", `?><?xml-stylesheet type="text/xsl" href="https://127.0.0.1/never-fetch"?><rss`, 1)
	items, err := Parse(t.Context(), []byte(withStyle))
	if err != nil || len(items) != 1 {
		t.Fatal("valid inert PI rejected", err)
	}
	// Zero entries, below byte/depth caps, but exceeds the independent token cap.
	manyTokens := `<rss version="2.0"><channel>` + strings.Repeat("<extension/>", 30000) + "</channel></rss>"
	if _, err = Parse(t.Context(), []byte(manyTokens)); err == nil {
		t.Fatal("token cap not enforced")
	}
}
