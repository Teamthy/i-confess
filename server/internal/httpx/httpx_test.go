package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRejectsTrailingDocuments(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{name: "single document", body: `{"ok":true}`, want: true},
		{name: "trailing object", body: `{"ok":true}{"extra":true}`, want: false},
		{name: "trailing scalar", body: `{"ok":true} null`, want: false},
		{name: "trailing garbage", body: `{"ok":true} nope`, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			var got map[string]bool
			err := DecodeJSON(r, &got)
			if (err == nil) != tc.want {
				t.Fatalf("DecodeJSON() error = %v, want success=%v", err, tc.want)
			}
		})
	}
}
