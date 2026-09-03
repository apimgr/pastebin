package compat

import (
	"context"
	"net/http"
)

// ─── subdomain-forced target (curl-upload family) ─────────────────────────────

// ctxTargetKey is the request-context key WithTarget/targetFromContext use to
// pass a subdomain-resolved compat target across the server/compat package
// boundary without an import cycle (server imports compat, never the reverse).
type ctxTargetKey struct{}

// WithTarget attaches target (as resolved by the server's compatModeGate
// middleware from the request's Host label — "termbin", "ixio", "sprunge", or
// "zerox0" for the curl-upload family; any other value, including one of the
// path-distinguishable compat targets, is simply ignored by RootUpload) to
// r's context. An empty target is a no-op so callers can pass through an
// unrecognized Host unchanged.
func WithTarget(r *http.Request, target string) *http.Request {
	if target == "" {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), ctxTargetKey{}, target))
}

// targetFromContext returns the subdomain-forced target for r, or "" when
// none was set.
func targetFromContext(r *http.Request) string {
	v, _ := r.Context().Value(ctxTargetKey{}).(string)
	return v
}

// TargetFromContextForTest exposes targetFromContext to other packages'
// tests (e.g. the server package's compatModeGate tests) so they can assert
// on the context wiring without an import cycle. Not used by production code.
func TargetFromContextForTest(r *http.Request) string {
	return targetFromContext(r)
}
