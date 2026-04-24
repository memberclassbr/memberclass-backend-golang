package member_import

import "github.com/go-chi/chi/v5"

// Register mounts the slice's HTTP routes. r is expected to already be scoped
// to `/imports` (the admin/frontend API prefix). CORS for this prefix is
// applied by the router when mounting /imports.
func (f *Feature) Register(r chi.Router, mw MiddlewareSet) {
	r.With(mw.SessionAuth).Post("/members", f.ImportMembers)
}
