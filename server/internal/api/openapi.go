package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// OpenAPI generation (§51).
//
// The document is built from the live route table, not maintained by hand. A
// hand-written spec is a second source of truth that drifts the moment someone
// adds an endpoint, and a client generated from a drifted spec fails in ways
// that look like backend bugs.

// OpenAPIVersion is the emitted spec version.
const OpenAPIVersion = "3.0.3"

// APIVersion is this API's version, surfaced in the spec and in generated
// client user agents.
const APIVersion = "1.0.0"

// OpenAPISpec builds the document.
func (h *Handler) OpenAPISpec(serverURL string) map[string]any {
	paths := map[string]any{}

	for _, r := range h.RouteTable() {
		// Static assets and SPA fallbacks are not part of the API contract.
		if isNonAPIRoute(r.Path) {
			continue
		}

		item, ok := paths[r.Path].(map[string]any)
		if !ok {
			item = map[string]any{}
			paths[r.Path] = item
		}

		op := map[string]any{
			"summary":     r.Summary,
			"operationId": operationID(r.Method, r.Path),
			"tags":        []string{r.Tag},
			"responses":   responsesFor(r),
		}
		if params := pathParams(r.Path); len(params) > 0 {
			op["parameters"] = params
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodPut {
			op["requestBody"] = map[string]any{
				"required": true,
				"content": map[string]any{
					"application/json": map[string]any{
						"schema": map[string]any{"type": "object"},
					},
				},
			}
		}
		if r.Auth != "public" {
			op["security"] = []any{map[string]any{"bearerAuth": []string{}}}
		}
		item[strings.ToLower(r.Method)] = op
	}

	return map[string]any{
		"openapi": OpenAPIVersion,
		"info": map[string]any{
			"title":   "i-confess API",
			"version": APIVersion,
			"description": "Generated from the live route table. " +
				"Audio is never served through this API: endpoints return short-lived signed URLs.",
		},
		"servers": []any{map[string]any{"url": serverURL}},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type": "http", "scheme": "bearer", "bearerFormat": "JWT",
				},
			},
			"schemas": errorSchemas(),
		},
		"paths": paths,
	}
}

// serveOpenAPI returns the spec.
//
// Public because a client generator needs it and it describes only the shape of
// endpoints, not their data. Anyone can discover the same routes by using the
// app.
func (h *Handler) serveOpenAPI(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if r.TLS == nil && strings.HasPrefix(r.Host, "localhost") {
		scheme = "http"
	}
	spec := h.OpenAPISpec(scheme + "://" + r.Host)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(spec)
}

// isNonAPIRoute filters out the SPA and media handlers.
func isNonAPIRoute(path string) bool {
	switch path {
	case "/", "/admin", "/admin/", "/media/":
		return true
	}
	return strings.HasPrefix(path, "/media/")
}

// operationID builds a stable identifier used as the generated method name.
//
// Stability matters: it becomes a function name in every generated client, and
// changing it is a breaking change for anyone who has written code against it.
func operationID(method, path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	for _, p := range parts {
		if strings.HasPrefix(p, "{") {
			b.WriteString("By")
			b.WriteString(title(strings.Trim(p, "{}")))
			continue
		}
		b.WriteString(title(p))
	}
	return b.String()
}

// pathParams describes {placeholders} in a path.
func pathParams(path string) []any {
	var out []any
	for _, seg := range strings.Split(path, "/") {
		if !strings.HasPrefix(seg, "{") || !strings.HasSuffix(seg, "}") {
			continue
		}
		name := strings.Trim(seg, "{}")
		out = append(out, map[string]any{
			"name": name, "in": "path", "required": true,
			"schema": map[string]any{"type": "string"},
		})
	}
	return out
}

// responsesFor describes the outcomes a client must handle.
//
// The error responses are listed deliberately: a generated client that only
// knows about 200 will surface a 402 as a generic failure, and the user gets
// "something went wrong" instead of "this is a Premium feature".
func responsesFor(r Route) map[string]any {
	res := map[string]any{
		"200": map[string]any{"description": "Success"},
	}
	if r.Method == http.MethodPost {
		res["201"] = map[string]any{"description": "Created"}
	}
	res["400"] = errorResponse("Invalid request")
	if r.Auth != "public" {
		res["401"] = errorResponse("Not authenticated, session revoked, or token reused")
		res["403"] = errorResponse("Authenticated but not permitted")
	}
	res["429"] = errorResponse("Rate limited; honour Retry-After")

	switch r.Tag {
	case "downloads", "sessions":
		res["402"] = errorResponse("Requires a subscription")
	}
	return res
}

func errorResponse(desc string) map[string]any {
	return map[string]any{
		"description": desc,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{"$ref": "#/components/schemas/Error"},
			},
		},
	}
}

// errorSchemas documents the error envelope every failure uses.
func errorSchemas() map[string]any {
	return map[string]any{
		"Error": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"error": map[string]any{
					"type":        "string",
					"description": "Human-readable message. Do not parse; use code.",
				},
				"code": map[string]any{
					"type": "string",
					"description": "Stable machine-readable code, e.g. AUTH_TOKEN_REUSED, " +
						"ENTITLEMENT_REQUIRED, USERNAME_TAKEN. Clients switch on this.",
				},
				"retry_after": map[string]any{
					"type":        "integer",
					"description": "Seconds to wait, present on 429.",
				},
			},
			"required": []string{"error"},
		},
	}
}

func title(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "_", " ")
	var b strings.Builder
	for _, word := range strings.Fields(s) {
		b.WriteString(strings.ToUpper(word[:1]))
		if len(word) > 1 {
			b.WriteString(word[1:])
		}
	}
	return b.String()
}
