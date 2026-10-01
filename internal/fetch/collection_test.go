package fetch

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCreatorFixtureEvergreenWithoutLive(t *testing.T) {
	b, e := os.ReadFile("../../testdata/creator.html")
	if e != nil {
		t.Fatal(e)
	}
	items, e := ExtractCreator(t.Context(), "https://fixture.example/creator", b, true)
	if e != nil {
		t.Fatal(e)
	}
	kinds := map[string]int{}
	for _, o := range items {
		kinds[o.Kind]++
		if o.State != "available" || o.AccountURL != "https://fixture.example/creator" || strings.Contains(o.OriginalURL, "chaturbate") {
			t.Fatal(o)
		}
	}
	if kinds["profile"] != 1 || kinds["image"] != 1 || kinds["video"] != 1 || kinds["link"] != 2 {
		t.Fatal(kinds)
	}
	if items[2].EmbedURL != "https://www.youtube-nocookie.com/embed/fixture1234" {
		t.Fatal(items)
	}
	noPreview, e := ExtractCreator(t.Context(), "https://fixture.example/creator", b, false)
	if e != nil {
		t.Fatal(e)
	}
	for _, o := range noPreview {
		if o.PreviewURL != "" || o.Kind == "image" {
			t.Fatal("preview permission escaped", o)
		}
	}
}
func TestCreatorMetadataCannotAuthorizeDestinations(t *testing.T) {
	body := []byte(`<meta property="og:title" content="Safe"><meta property="og:image" content="http://127.0.0.1/x"><a rel="me" href="javascript:alert(1)">Bad</a><script type="application/ld+json">{"@type":"VideoObject","name":"Unsafe","url":"https://video.example/a","embedUrl":"https://evil.example/embed/fixture1234"}</script>`)
	items, e := ExtractCreator(t.Context(), "https://fixture.example/creator", body, true)
	if e != nil {
		t.Fatal(e)
	}
	if len(items) != 2 || items[1].EmbedURL != "" || items[1].Display != "link" {
		t.Fatal(items)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e = ExtractCreator(ctx, "https://fixture.example/creator", body, true); e == nil {
		t.Fatal("cancel ignored")
	}
	if _, e = ExtractCreator(t.Context(), "https://fixture.example/creator", []byte(strings.Repeat("<b>", 60001)), true); e == nil {
		t.Fatal("token cap ignored")
	}
}
