package security

// Checks — §79 security tests: auth/authz, IDOR, rate-limit, token abuse.
// This file lists the test cases that CI must enforce; actual tests live in
// server/internal/api/*_test.go and are run with `go test -race`.

var Cases = []Case{
	{Name: "auth_no_token_denied", Kind: "authz", Expect: 401},
	{Name: "auth_forged_premium_denied", Kind: "entitlement", Expect: 402, Note: "client forged premium:true → still 402 (billing security §6.5)"},
	{Name: "idor_userA_cannot_read_userB_session", Kind: "idor", Expect: 404},
	{Name: "idor_userA_cannot_delete_userB_template", Kind: "idor", Expect: 403},
	{Name: "rate_limit_login_per_ip", Kind: "rate_limit", Expect: 429},
	{Name: "rate_limit_register_per_ip", Kind: "rate_limit", Expect: 429},
	{Name: "token_reuse_detected", Kind: "token", Expect: 401, Note: "refresh token replay → revocation"},
	{Name: "session_fixation_rotated", Kind: "session", Expect: 200, Note: "login rotates session ID"},
	{Name: "sql_injection_categories_search", Kind: "sqli", Expect: 200, Note: "tsvector sanitized, no 500"},
	{Name: "xss_confession_body_escaped", Kind: "xss", Expect: 200, Note: "body rendered escaped"},
}

type Case struct {
	Name   string
	Kind   string
	Expect int
	Note   string
}
