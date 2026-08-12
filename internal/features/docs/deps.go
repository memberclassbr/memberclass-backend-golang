// Package docs is a vertical slice serving the OpenAPI spec and the Swagger UI
// under `/docs`. The spec ships embedded in the binary as a text/template so a
// single image can be deployed per workspace: the brand name and the public API
// URL come from the environment at request time, not from hardcoded strings.
//
// See CLAUDE.md ("Architecture migration in progress") for the target structure
// and rules for new features during the migration.
package docs

import (
	"github.com/memberclass-backend-golang/internal/domain/ports"
)

// Feature holds the shared dependencies for every action in this slice.
type Feature struct {
	log ports.Logger
}

// New builds the slice. Wire it in cmd/api/main.go via fx.Provide.
//
// The workspace branding is validated once here so a misconfigured deploy is
// visible in the startup logs instead of only in the rendered docs.
func New(log ports.Logger) *Feature {
	f := &Feature{log: log}

	if apiNameFromEnv() == "" {
		log.Warn("docs.api_name_missing",
			"hint", "set PUBLIC_API_NAME to this workspace's brand (e.g. MemberClass); docs render unbranded until then",
		)
	}
	if apiBaseURLFromEnv() == "" {
		log.Warn("docs.api_url_missing",
			"hint", "set PUBLIC_API_URL to the public API base URL (e.g. https://api.memberclass.com.br); falling back to the request host",
		)
	}

	return f
}
