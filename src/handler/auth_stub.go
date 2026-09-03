package handler

import "net/http"

// AuthStubRedirect redirects any auth web route (login/register/logout/settings) to /.
func AuthStubRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusFound)
}
