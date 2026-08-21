package apikeyusage

import "strings"

// apiPrefix is stripped from every pattern. Every route that authenticates
// with a tenant API key is mounted below it, so keeping it would put the same
// eleven characters on every row of the panel.
const apiPrefix = "/api/v1/"

// Endpoint turns a chi route pattern into the stable identifier stored in
// "ApiKeyUsageDaily"."endpoint": /api/v1/comments/{commentId} becomes
// comments/:commentId.
//
// The colon form is not a preference. The panel was written against that
// spelling, and two spellings of one endpoint read as two endpoints.
//
// It returns "" for anything without a usable pattern — a 404, a 405, or a
// mount matched by wildcard. Those requests are still rate limited and still
// answered; they just do not become a row, because the only identifier
// available for them is the raw path, and a row per id is the cardinality the
// pattern exists to avoid.
func Endpoint(pattern string) string {
	if pattern == "" || strings.Contains(pattern, "*") {
		return ""
	}

	trimmed := strings.TrimPrefix(pattern, apiPrefix)
	if trimmed == pattern {
		trimmed = strings.TrimPrefix(pattern, "/")
	}
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return ""
	}

	// {commentId} -> :commentId. A pattern with an unbalanced brace is not one
	// chi produced, so it is refused rather than half-converted.
	var b strings.Builder
	b.Grow(len(trimmed))
	for i := 0; i < len(trimmed); i++ {
		switch trimmed[i] {
		case '{':
			end := strings.IndexByte(trimmed[i:], '}')
			if end < 0 {
				return ""
			}
			// chi allows a regexp inside the brace ({id:[0-9]+}); only the
			// name belongs in the identifier.
			name := trimmed[i+1 : i+end]
			if colon := strings.IndexByte(name, ':'); colon >= 0 {
				name = name[:colon]
			}
			b.WriteByte(':')
			b.WriteString(name)
			i += end
		case '}':
			return ""
		default:
			b.WriteByte(trimmed[i])
		}
	}

	return b.String()
}
