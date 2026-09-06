package db

import (
	"strconv"
	"strings"
)

// Rebind rewrites SQLite-style `?` placeholders into PostgreSQL's positional
// `$N` form.
//
// The store layer was written against SQLite and carries 663 `?` parameters
// across 14 files. Rewriting each one by hand would produce an unreviewable
// diff and silently break whenever someone later copies a query. Rebinding at
// one choke point keeps the SQL readable in the form it was written.
//
// The scanner is aware of string literals, comments and dollar-quoted blocks,
// because a naive replace is not merely untidy — it is wrong in two ways:
//
//   - A `?` inside a quoted value is data. Renumbering it corrupts the query.
//   - A `?` inside a comment is ignored by the server, but incrementing the
//     counter for it shifts every subsequent real placeholder by one, so the
//     query binds arguments to the wrong positions. That failure looks like a
//     data bug, not a syntax bug, which makes it expensive to find.
//
// Queries that already use `$N` are left alone; the two forms must not be
// mixed in one statement.
func Rebind(query string) string {
	if !strings.ContainsRune(query, '?') {
		return query
	}

	var b strings.Builder
	b.Grow(len(query) + 16)

	n := 0
	for i := 0; i < len(query); {
		switch c := query[i]; {
		case c == '\'':
			i = copySingleQuoted(&b, query, i)
		case c == '`':
			// Not valid PostgreSQL, but SQLite identifiers use backticks and a
			// stray one must not be mistaken for a placeholder context.
			i = copyDelimited(&b, query, i, '`')
		case c == '"':
			i = copyDelimited(&b, query, i, '"')
		case c == '$':
			if tag, ok := dollarTag(query, i); ok {
				i = copyDollarQuoted(&b, query, i, tag)
			} else {
				b.WriteByte(c)
				i++
			}
		case c == '-' && i+1 < len(query) && query[i+1] == '-':
			for i < len(query) && query[i] != '\n' {
				b.WriteByte(query[i])
				i++
			}
		case c == '/' && i+1 < len(query) && query[i+1] == '*':
			b.WriteString("/*")
			i += 2
			for i < len(query) {
				if query[i] == '*' && i+1 < len(query) && query[i+1] == '/' {
					b.WriteString("*/")
					i += 2
					break
				}
				b.WriteByte(query[i])
				i++
			}
		case c == '?':
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// copySingleQuoted copies a '...' literal, honouring ” as an escaped quote.
func copySingleQuoted(b *strings.Builder, s string, i int) int {
	b.WriteByte('\'')
	i++
	for i < len(s) {
		if s[i] == '\'' {
			b.WriteByte('\'')
			i++
			// Two consecutive quotes are one literal quote, not the end.
			if i < len(s) && s[i] == '\'' {
				b.WriteByte('\'')
				i++
				continue
			}
			return i
		}
		b.WriteByte(s[i])
		i++
	}
	return i
}

// copyDelimited copies a run enclosed in the same single-byte delimiter.
func copyDelimited(b *strings.Builder, s string, i int, delim byte) int {
	b.WriteByte(delim)
	i++
	for i < len(s) {
		b.WriteByte(s[i])
		if s[i] == delim {
			i++
			return i
		}
		i++
	}
	return i
}

// dollarTag recognises the opening of a dollar-quoted string: $$ or $tag$.
// Tags follow identifier rules and cannot contain a dollar sign.
func dollarTag(s string, i int) (string, bool) {
	if i >= len(s) || s[i] != '$' {
		return "", false
	}
	for j := i + 1; j < len(s); j++ {
		c := s[j]
		switch {
		case c == '$':
			return s[i : j+1], true
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (j > i+1 && c >= '0' && c <= '9'):
			continue
		default:
			return "", false
		}
	}
	return "", false
}

// copyDollarQuoted copies $tag$ ... $tag$ verbatim, including any '?' inside.
func copyDollarQuoted(b *strings.Builder, s string, i int, tag string) int {
	b.WriteString(tag)
	i += len(tag)
	if end := strings.Index(s[i:], tag); end >= 0 {
		b.WriteString(s[i : i+end+len(tag)])
		return i + end + len(tag)
	}
	b.WriteString(s[i:])
	return len(s)
}
