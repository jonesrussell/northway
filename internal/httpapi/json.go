package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxRequestBytes = 32 << 10

var errJSON = errors.New("invalid JSON request")

// readObject bounds bytes and depth before allocating a typed DTO. Token parsing
// rejects duplicate decoded keys at every depth (including escaped aliases).
// Shape checks below are case-sensitive; encoding/json's struct-key folding is
// never used as validation. Missing/null optional fields remain distinguishable.
func readObject(w http.ResponseWriter, r *http.Request) (map[string]any, error) {
	if len(r.Header.Values("Content-Type")) != 1 || len(r.Header.Values("Content-Encoding")) != 0 || r.URL.RawQuery != "" {
		return nil, errJSON
	}
	media, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return nil, errJSON
	}
	for k, v := range params {
		if k != "charset" || !strings.EqualFold(v, "utf-8") {
			return nil, errJSON
		}
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil || !utf8.Valid(b) || !validSurrogates(b) {
		return nil, errJSON
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	value, err := jsonValue(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, errJSON
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errJSON
	}
	return object, nil
}
func jsonValue(d *json.Decoder, depth int) (any, error) {
	if depth > 8 {
		return nil, errJSON
	}
	token, err := d.Token()
	if err != nil {
		return nil, errJSON
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		out := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, errJSON
			}
			k, ok := key.(string)
			if !ok {
				return nil, errJSON
			}
			if _, exists := out[k]; exists {
				return nil, errJSON
			}
			v, err := jsonValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			out[k] = v
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return nil, errJSON
		}
		return out, nil
	case '[':
		out := []any{}
		for d.More() {
			v, err := jsonValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return nil, errJSON
		}
		return out, nil
	default:
		return nil, errJSON
	}
}
func fields(m map[string]any, required, optional []string) bool {
	for _, k := range required {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	for k, v := range m {
		if v == nil || (!slices.Contains(required, k) && !slices.Contains(optional, k)) {
			return false
		}
	}
	return true
}
func stringField(m map[string]any, k string) bool { v, ok := m[k].(string); return ok && v != "" }
func optionalStrings(m map[string]any, k string) bool {
	v, exists := m[k]
	if !exists {
		return true
	}
	a, ok := v.([]any)
	if !ok {
		return false
	}
	for _, s := range a {
		if _, ok := s.(string); !ok {
			return false
		}
	}
	return true
}
func queryShape(m map[string]any) bool {
	if !fields(m, []string{"feed_id", "context", "max_age_hours", "limit"}, nil) || !stringField(m, "feed_id") {
		return false
	}
	c, ok := m["context"].(map[string]any)
	if !ok || !fields(c, []string{"intent", "technologies"}, []string{"focus"}) || !stringField(c, "intent") || !optionalStrings(c, "focus") {
		return false
	}
	a, ok := c["technologies"].([]any)
	if !ok || len(a) > 32 {
		return false
	}
	for _, v := range a {
		t, ok := v.(map[string]any)
		if !ok || !fields(t, []string{"name"}, []string{"version"}) || !stringField(t, "name") {
			return false
		}
		if _, ok := t["version"]; ok && !stringField(t, "version") {
			return false
		}
	}
	// JSON Schema integers include 1.0 and 1e0. Normalize exact bounded integers
	// without float rounding before decoding the existing typed request.
	for _, k := range []string{"max_age_hours", "limit"} {
		n, ok := m[k].(json.Number)
		if !ok || len(n) > 100 {
			return false
		}
		// Bound exponent expansion before math/big allocation. With <=100 bytes,
		// larger exponents cannot represent an accepted positive integer <=720.
		if i := strings.IndexAny(string(n), "eE"); i >= 0 {
			exponent, err := strconv.Atoi(string(n)[i+1:])
			if err != nil || exponent < -100 || exponent > 100 {
				return false
			}
		}
		v, ok := new(big.Rat).SetString(string(n))
		if !ok || !v.IsInt() || !v.Num().IsInt64() {
			return false
		}
		value := v.Num().Int64()
		maximum := int64(720)
		if k == "limit" {
			maximum = 20
		}
		if value < 1 || value > maximum {
			return false
		}
		m[k] = value
	}
	return true
}
func feedbackShape(m map[string]any) bool {
	if !fields(m, []string{"event_id", "snapshot_id", "article_id", "action"}, []string{"reverses_event_id"}) {
		return false
	}
	for k := range m {
		if !stringField(m, k) {
			return false
		}
	}
	_, reversal := m["reverses_event_id"]
	return (m["action"] == "undo") == reversal
}
func decodeObject(m map[string]any, dst any) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// encoding/json replaces unpaired UTF-16 surrogates with U+FFFD. Reject that
// lossy input instead, while allowing escaped backslashes and valid pairs.
func validSurrogates(b []byte) bool {
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' {
			continue
		}
		i++
		if i >= len(b) {
			return false
		}
		if b[i] != 'u' {
			continue
		}
		if i+4 >= len(b) {
			return false
		}
		v, err := strconv.ParseUint(string(b[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if v >= 0xdc00 && v <= 0xdfff {
			return false
		}
		if v >= 0xd800 && v <= 0xdbff {
			if i+6 >= len(b) || b[i+1] != '\\' || b[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(b[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
