package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// EncodeJSON writes v as JSON without setting an HTTP status.
func EncodeJSON(w http.ResponseWriter, v any) error {
	return json.NewEncoder(w).Encode(v)
}

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = EncodeJSON(w, v)
}

// WriteError writes a consistent error envelope.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// DecodeJSON decodes the request body, enforcing a size limit and single value.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		return err
	}
	// Decode one more value to ensure the body contains exactly one JSON
	// document. dec.More only has meaning while decoding an array or object;
	// using it here allowed payloads such as `{"ok":true}{"extra":true}` to
	// pass silently.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("unexpected trailing JSON")
		}
		return err
	}
	return nil
}
