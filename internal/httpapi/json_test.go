package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStrictUnicode(t *testing.T) {
	for _, s := range []string{`{"value":"\ud83d\ude00"}`, `{"value":"\\ud800"}`, `{"value":"🌎"}`} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(s))
		r.Header.Set("Content-Type", "application/json")
		if _, err := readObject(httptest.NewRecorder(), r); err != nil {
			t.Fatal("valid Unicode rejected", s, err)
		}
	}
}
func FuzzBoundedJSON(f *testing.F) {
	for _, s := range []string{`{}`, `{"a":1,"a":2}`, `{"a":[null,{"a":1}]}`, `{"x":"\ud800"}`, `null`, strings.Repeat(`[`, 12)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > maxRequestBytes+1 {
			return
		}
		r := httptest.NewRequest("POST", "/", strings.NewReader(s))
		r.Header.Set("Content-Type", "application/json")
		object, err := readObject(httptest.NewRecorder(), r)
		if err == nil {
			queryShape(object)
			feedbackShape(object)
		}
	})
}
