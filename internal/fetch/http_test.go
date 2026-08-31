package fetch

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonesrussell/northway/internal/ingest"
)

type fakeResolver struct {
	ips   []netip.Addr
	calls int
}

func (r *fakeResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	r.calls++
	return r.ips, nil
}
func TestAddressPolicy(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1", "192.168.1.1", "10.0.0.1", "169.254.169.254", "100.100.100.200", "224.0.0.1", "198.18.0.1", "2001:db8::1", "64:ff9b::7f00:1", "fe80::1", "fc00::1", "2002:7f00:1::"} {
		if publicIP(netip.MustParseAddr(ip)) {
			t.Fatal("allowed", ip)
		}
	}
	for _, ip := range []string{"1.1.1.1", "2606:4700:4700::1111"} {
		if !publicIP(netip.MustParseAddr(ip)) {
			t.Fatal("blocked", ip)
		}
	}
	for _, raw := range []string{"http://example.invalid/feed", "https://user@example.invalid/feed", "https://example.invalid:444/feed", "https://example.invalid/feed?q=1", "https://example.invalid/feed#x", "https://example.invalid./feed"} {
		if _, err := SourceURL(raw); err == nil {
			t.Fatal("accepted", raw)
		}
	}
}

func TestMixedDNSFailsBeforeDial(t *testing.T) {
	r := &fakeResolver{ips: []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("127.0.0.1")}}
	c := New()
	c.resolver = r
	c.dial = func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("dialed unsafe answer")
		return nil, errors.New("no")
	}
	result := c.Fetch(t.Context(), ingest.Claim{URL: "https://example.invalid/feed", MaxBytes: 2048})
	if result.Failure != "transport" {
		t.Fatal(result)
	}
}

func TestPinnedTLSConditionalAndNoTraversal(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/feed" {
			t.Error("followed non-feed path", r.URL.Path)
		}
		if r.Header.Get("Accept-Encoding") != "identity" || r.UserAgent() == "" {
			t.Error("missing client policy")
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		io.WriteString(w, rss)
	}))
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	resolver := &fakeResolver{ips: []netip.Addr{netip.MustParseAddr("1.1.1.1")}}
	c := New()
	c.resolver = resolver
	c.roots = roots
	c.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != "1.1.1.1:443" {
			t.Error("address was not pinned", address)
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	// httptest's certificate includes example.com, retaining real verification.
	claim := ingest.Claim{URL: "https://example.com/feed", MaxBytes: 2048}
	first := c.Fetch(t.Context(), claim)
	if first.Failure != "" || len(first.Items) != 1 || first.ETag != `"v1"` {
		t.Fatal(first)
	}
	claim.ETag = first.ETag
	second := c.Fetch(t.Context(), claim)
	if second.Status != 304 || second.Failure != "" || len(second.Items) != 0 {
		t.Fatal(second)
	}
	if requests.Load() != 2 || resolver.calls != 2 {
		t.Fatal("traversal, retries or stale DNS", requests.Load(), resolver.calls)
	}
}

func TestRedirectAndTLSFailClosed(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		http.Redirect(w, r, "https://127.0.0.1/secret", 302)
	}))
	defer srv.Close()
	c := New()
	c.resolver = &fakeResolver{ips: []netip.Addr{netip.MustParseAddr("1.1.1.1")}}
	c.dial = func(ctx context.Context, n, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, n, srv.Listener.Addr().String())
	}
	claim := ingest.Claim{URL: "https://example.com/feed", MaxBytes: 2048}
	if r := c.Fetch(t.Context(), claim); r.Failure != "transport" {
		t.Fatal("untrusted TLS accepted", r)
	}
	c.roots = x509.NewCertPool()
	c.roots.AddCert(srv.Certificate())
	if r := c.Fetch(t.Context(), claim); r.Status != 302 || r.Failure != "http" {
		t.Fatal(r)
	}
	if count.Load() != 1 {
		t.Fatal("followed redirect or retried", count.Load())
	}
}

type brokenReader struct{ done bool }

func (r *brokenReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errors.New("synthetic partial failure")
	}
	r.done = true
	return copy(p, []byte("<rss>")), nil
}
func TestResponseBoundsAndHolds(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		body    io.Reader
		headers http.Header
		limit   int64
		failure string
	}{
		{"oversize", strings.NewReader(strings.Repeat("x", 4096)), http.Header{}, 1024, "body"},
		{"compressed", strings.NewReader(rss), http.Header{"Content-Encoding": []string{"gzip"}}, 2048, "encoding"},
		{"partial", &brokenReader{}, http.Header{}, 2048, "body"},
		{"no-store", strings.NewReader(rss), http.Header{"Cache-Control": []string{"no-store"}}, 2048, "no_store"},
		{"malformed", strings.NewReader("<rss>"), http.Header{}, 2048, "parse"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := readResponse(t.Context(), &http.Response{StatusCode: 200, Header: tc.headers, Body: io.NopCloser(tc.body)}, tc.limit, now)
			if r.Failure != tc.failure || r.Bytes > tc.limit || len(r.Items) != 0 {
				t.Fatal(r)
			}
		})
	}
	h := http.Header{"Retry-After": []string{"7200"}, "Cache-Control": []string{"max-age=10800"}, "Age": []string{"3600"}}
	if got := notBefore(h, now); !got.Equal(now.Add(2 * time.Hour)) {
		t.Fatal(got)
	}
	h.Set("Retry-After", "99999999999999999999999999")
	if got := notBefore(h, now); got.Before(now.Add(7 * 24 * time.Hour)) {
		t.Fatal("overflow shortened hold", got)
	}
}

func TestFetchDeadlineStopsBodyWithoutRetry(t *testing.T) {
	var count atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		io.WriteString(w, "<rss>")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	c := New()
	c.resolver = &fakeResolver{ips: []netip.Addr{netip.MustParseAddr("1.1.1.1")}}
	c.roots = x509.NewCertPool()
	c.roots.AddCert(srv.Certificate())
	c.dial = func(ctx context.Context, n, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, n, srv.Listener.Addr().String())
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	result := c.Fetch(ctx, ingest.Claim{URL: "https://example.com/feed", MaxBytes: 2048})
	if result.Failure == "" || count.Load() != 1 || time.Since(start) > time.Second {
		t.Fatal(result, count.Load())
	}
}
