package fetch

// Conditional header and 304 behavior adapted from North Cloud
// crawler/internal/feed/http_fetcher.go at 51b877de7dab311c981dcdb4d38dfdca9965aeb1.
import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jonesrussell/northway/internal/ingest"
)

type resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}
type Client struct {
	roots *x509.CertPool // nil in production; test-only TLS trust seam

	resolver resolver
	dial     func(context.Context, string, string) (net.Conn, error)
}

func New() *Client {
	d := &net.Dialer{Timeout: 5 * time.Second}
	return &Client{resolver: net.DefaultResolver, dial: d.DialContext}
}

var excluded = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("169.254.0.0/16"), netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("3fff::/20"),
}

func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.Zone() != "" {
		return false
	}
	// Only currently allocated global IPv6 unicast; excludes NAT64/ULA/link-local.
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, p := range excluded {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

func SourceURL(raw string) (*url.URL, error) {
	value, err := ItemURL(raw)
	if err != nil || value != raw {
		return nil, ingest.ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.RawQuery != "" || u.ForceQuery || strings.HasSuffix(u.Hostname(), ".") {
		return nil, ingest.ErrInvalid
	}
	return u, nil
}

func header(v string) string {
	if len(v) > 1024 || strings.ContainsAny(v, "\r\n\x00") {
		return ""
	}
	return v
}

func notBefore(h http.Header, now time.Time) time.Time {
	var until time.Time
	set := func(t time.Time) {
		if t.After(now) && t.After(until) {
			until = t
		}
	}
	if v := h.Get("Retry-After"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			if n > int64((1<<63-1)/int64(time.Second)) {
				set(time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
			} else {
				set(now.Add(time.Duration(n) * time.Second))
			}
		} else if t, err := http.ParseTime(v); err == nil {
			set(t)
		} else {
			set(now.Add(7 * 24 * time.Hour))
		}
	}
	// Fail closed on excessive or malformed holds: require operator review;
	// FinishPoll preserves longer representable absolute Retry-After dates.
	age, ageErr := strconv.ParseInt(h.Get("Age"), 10, 64)
	if ageErr != nil {
		age = 0
	}
	age = max(age, 0)
	for _, part := range strings.Split(strings.Join(h.Values("Cache-Control"), ","), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && strings.EqualFold(key, "max-age") {
			n, err := strconv.ParseInt(strings.Trim(value, "\""), 10, 64)
			if err != nil || n < 0 {
				set(now.Add(7 * 24 * time.Hour))
			}
			if err == nil && n > age {
				if n-age > int64((1<<63-1)/int64(time.Second)) {
					set(time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
				} else {
					set(now.Add(time.Duration(n-age) * time.Second))
				}
			}
		}
	}
	if t, err := http.ParseTime(h.Get("Expires")); err == nil {
		set(t)
	}
	return until
}

func (c *Client) Fetch(ctx context.Context, claim ingest.Claim) ingest.Result {
	result := ingest.Result{Failure: "transport"}
	if ctx.Err() != nil || claim.MaxBytes < 1 || claim.MaxBytes > ingest.MaxResponseBytes {
		return result
	}
	u, err := SourceURL(claim.URL)
	if err != nil {
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, ingest.FetchTimeout)
	defer cancel()
	ips, err := c.resolver.LookupNetIP(ctx, "ip", u.Hostname())
	if err != nil || len(ips) == 0 {
		return result
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return result
		}
	}
	// One pinned address, one attempt, no proxy, redirects, connection reuse or
	// HTTP/2 stream retries. TLS verifies the original publisher hostname.
	address := net.JoinHostPort(ips[0].Unmap().String(), "443")
	tr := &http.Transport{Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 16384,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, ServerName: u.Hostname(), RootCAs: c.roots},
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		DialContext:  func(ctx context.Context, _, _ string) (net.Conn, error) { return c.dial(ctx, "tcp", address) },
		TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: ingest.FetchTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return result
	}
	req.Header.Set("User-Agent", "Northway/0.1 (+https://github.com/jonesrussell/northway)")
	req.Header.Set("Accept", "application/atom+xml, application/rss+xml, application/xml, text/xml")
	req.Header.Set("Accept-Encoding", "identity")
	if v := header(claim.ETag); v != "" {
		req.Header.Set("If-None-Match", v)
	}
	if v := header(claim.LastModified); v != "" {
		req.Header.Set("If-Modified-Since", v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return result
	}
	defer resp.Body.Close()
	if resp.StatusCode == 304 && header(claim.ETag) == "" && header(claim.LastModified) == "" {
		return ingest.Result{Status: 304, Failure: "http"}
	}
	return readResponse(ctx, resp, claim.MaxBytes, time.Now().UTC())
}

func readResponse(ctx context.Context, resp *http.Response, limit int64, now time.Time) ingest.Result {
	result := ingest.Result{Status: resp.StatusCode, ETag: header(resp.Header.Get("ETag")), LastModified: header(resp.Header.Get("Last-Modified")), NotBefore: notBefore(resp.Header, now)}
	if resp.StatusCode == http.StatusNotModified {
		return result
	}
	if resp.StatusCode != http.StatusOK {
		result.Failure = "http"
		return result
	}
	if enc := resp.Header.Get("Content-Encoding"); enc != "" && !strings.EqualFold(enc, "identity") {
		result.Failure = "encoding"
		return result
	}
	// No decompression means the wire-body and decoded bounds coincide. Read no
	// more than the reservation; a response exactly at the cap is rejected because
	// proving EOF would consume another byte. There is no uncharged overflow read.
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx, resp.Body}, limit))
	result.Bytes = int64(len(data))
	if err != nil || result.Bytes >= limit || resp.ContentLength > limit {
		result.Failure = "body"
		return result
	}
	for _, part := range strings.Split(strings.Join(resp.Header.Values("Cache-Control"), ","), ",") {
		if strings.EqualFold(strings.TrimSpace(part), "no-store") {
			result.Failure = "no_store"
			return result
		}
	}
	items, err := Parse(ctx, data)
	if err != nil {
		result.Failure = "parse"
		return result
	}
	result.Items = items
	return result
}

var _ ingest.Fetcher = (*Client)(nil)
