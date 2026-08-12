package docs

import (
	"bytes"
	_ "embed"
	"html"
	"net/http"
	"os"
	"strings"
	"text/template"
)

// swaggerSpecSource is the OpenAPI spec, embedded at build time. It is a
// text/template, not a finished document: every workspace-specific value is a
// quoted `{{.Field}}` placeholder (quoted so the file stays valid YAML for
// editors and linters).
//
//go:embed swagger.yaml
var swaggerSpecSource string

// Parsed once at init. A malformed placeholder is a boot-time panic rather
// than a per-request surprise — the render test in this package is what keeps
// that from reaching a deploy.
var swaggerSpecTemplate = template.Must(template.New("swagger").Parse(swaggerSpecSource))

// Fallbacks used when the corresponding env var is empty. They are deliberately
// brand-neutral: a workspace that forgets to configure itself must render as
// generic, never as another workspace.
const (
	defaultAPIName   = "API"
	defaultFilesURL  = "https://files.example.com"
	defaultDomainURL = "example.com"
)

// specVars are the per-workspace values injected into swagger.yaml.
type specVars struct {
	// APIName is the workspace brand shown in the description and contact
	// block. From PUBLIC_API_NAME.
	APIName string
	// APITitle is the document title. Computed, not an env var: an
	// unconfigured workspace renders "API" rather than "API API".
	APITitle string
	// APIBaseURL is the public base URL advertised in `servers[]`. From
	// PUBLIC_API_URL, falling back to the scheme+host of the current request.
	APIBaseURL string
	// FilesURL is the CDN prefix used by response examples. From
	// PUBLIC_FILES_URL (or NEXT_PUBLIC_FILES_URL).
	FilesURL string
	// DomainURL is the customer-facing frontend root used by response
	// examples. From PUBLIC_DOMAIN_URL (or NEXT_PUBLIC_DOMAIN_URL).
	DomainURL string
}

func (f *Feature) ServeSwaggerYAML(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := swaggerSpecTemplate.Execute(&buf, buildSpecVars(r)); err != nil {
		f.log.Error("docs.spec_render_failed", "error", err)
		http.Error(w, "Failed to render OpenAPI spec", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(buf.Bytes())
}

func (f *Feature) ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	vars := buildSpecVars(r)
	specURL := requestBaseURL(r) + "/docs/swagger.yaml"

	page := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + html.EscapeString(vars.APITitle) + ` - Swagger UI</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css">
    <style>
        html { box-sizing: border-box; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; background: #fafafa; }
        .topbar { display: none; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: "` + specURL + `",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIStandalonePreset
                ],
                plugins: [
                    SwaggerUIBundle.plugins.DownloadUrl
                ],
                layout: "StandaloneLayout"
            });
        };
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(page))
}

// buildSpecVars resolves the workspace branding for this request. Every field
// falls back to a neutral placeholder, so the rendered spec is always a
// well-formed document even on a half-configured deploy.
func buildSpecVars(r *http.Request) specVars {
	apiName := apiNameFromEnv()
	apiTitle := apiName + " API"
	if apiName == "" {
		apiName = defaultAPIName
		apiTitle = defaultAPIName
	}

	baseURL := apiBaseURLFromEnv()
	if baseURL == "" {
		baseURL = requestBaseURL(r)
	}

	filesURL := strings.TrimRight(firstNonEmpty(
		os.Getenv("PUBLIC_FILES_URL"),
		os.Getenv("NEXT_PUBLIC_FILES_URL"),
	), "/")
	if filesURL == "" {
		filesURL = defaultFilesURL
	}

	domainURL := firstNonEmpty(
		os.Getenv("PUBLIC_DOMAIN_URL"),
		os.Getenv("NEXT_PUBLIC_DOMAIN_URL"),
	)
	if domainURL == "" {
		domainURL = defaultDomainURL
	}

	return specVars{
		APIName:    apiName,
		APITitle:   apiTitle,
		APIBaseURL: baseURL,
		FilesURL:   filesURL,
		DomainURL:  domainURL,
	}
}

func apiNameFromEnv() string {
	return strings.TrimSpace(os.Getenv("PUBLIC_API_NAME"))
}

// apiBaseURLFromEnv returns PUBLIC_API_URL without a trailing slash — the spec
// paths already start with `/`, and `https://host//api/v1` is an ugly (and in
// some clients, broken) request URL.
func apiBaseURLFromEnv() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_API_URL")), "/")
}

// requestBaseURL rebuilds the public scheme+host the caller used, honouring the
// proxy header since the API always runs behind one in production.
func requestBaseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		scheme = forwardedProto
	}

	return scheme + "://" + r.Host
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
