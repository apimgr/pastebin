package server

import (
	"context"
	"net/http"
	"strings"
)

// ctxKeyAuthToken carries the bearer/API token presented with the request.
const ctxKeyAuthToken ctxKeyType = 101

// ctxKeyAuthSource records which header (or the query string) the token in
// ctxKeyAuthToken came from, for audit logging and metric labelling.
const ctxKeyAuthSource ctxKeyType = 102

// authTokenHeaders lists every header the server accepts a token from, in the
// exact priority order PART 8 defines (AI.md:12946-12951). Case variants of the
// same logical header sit next to each other because Go canonicalises header
// names on lookup; they are listed for documentation value.
var authTokenHeaders = []string{
	"Authorization",
	"X-API-Key",
	"API-Key",
	"ApiKey",
	"X-Auth-Token",
	"X-Access-Token",
	"X-Token",
	"Token",
	"X-Service-Token",
	"X-Internal-Token",
}

// extractRequestToken returns the token presented with the request and the
// name of the source it was taken from ("authorization", the header name, or
// "query"). It returns empty strings when no token is present. Cookies are
// deliberately never consulted: API routes must not honour ambient authority
// (PART 8), and the web routes that do accept the owner-token cookie read it
// explicitly in their own handlers.
func extractRequestToken(r *http.Request) (string, string) {
	for _, name := range authTokenHeaders {
		v := strings.TrimSpace(r.Header.Get(name))
		if v == "" {
			continue
		}
		if name == "Authorization" {
			// Only the Bearer scheme carries a token this server can validate;
			// Basic/Digest credentials are not a supported auth method here.
			const prefix = "bearer "
			if len(v) > len(prefix) && strings.EqualFold(v[:len(prefix)], prefix) {
				if tok := strings.TrimSpace(v[len(prefix):]); tok != "" {
					return tok, "authorization"
				}
			}
			continue
		}
		return v, strings.ToLower(name)
	}
	if tok := strings.TrimSpace(r.URL.Query().Get("token")); tok != "" {
		return tok, "query"
	}
	return "", ""
}

// TokenFromContext returns the token attached by authMiddleware and the source
// it was read from. Both are empty when the request presented no credentials.
func TokenFromContext(ctx context.Context) (string, string) {
	tok, _ := ctx.Value(ctxKeyAuthToken).(string)
	src, _ := ctx.Value(ctxKeyAuthSource).(string)
	return tok, src
}

// authMiddleware is execution step 9 of the canonical middleware chain
// (AI.md:7180). It only resolves the credential presented with the request into
// the request context — it never rejects a request. Authorization is performed
// per route by requireOperatorToken and the resource-owner checks, so that
// unauthenticated public routes keep working unchanged.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, source := extractRequestToken(r)
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyAuthToken, token)
		ctx = context.WithValue(ctx, ctxKeyAuthSource, source)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
