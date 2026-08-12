// Package docs serves the hand-maintained OpenAPI spec and a Swagger UI for it
// at /docs.
//
// The spec is embedded in the binary as a text/template rather than shipped as
// a file, because one deployment per customer means the brand name and the
// public API host differ per deployment. Every workspace-specific value is a
// quoted `{{.Field}}` placeholder resolved from config at request time — the
// same image serves every workspace.
//
// The fallbacks are deliberately brand-neutral: a deployment that has not been
// told who it is renders generic docs, never another customer's brand.
package docs

import (
	"bytes"
	_ "embed"
	"html"
	"net/http"
	"strings"
	"text/template"

	"github.com/go-chi/chi/v5"
	"github.com/memberclass-backend-golang/internal/platform/config"
	"github.com/memberclass-backend-golang/internal/platform/logger"
)

//go:embed swagger.yaml
var swaggerSpecSource string

// Parsed once at init: a malformed placeholder is a boot-time panic rather than
// a per-request surprise. The render test in this package is what keeps such a
// typo from reaching a deployment.
var swaggerSpecTemplate = template.Must(template.New("swagger").Parse(swaggerSpecSource))

// Fallbacks for an unconfigured deployment. config.Load already warns about the
// missing variables; these keep the rendered document well-formed meanwhile.
const (
	defaultAPIName  = "API"
	defaultFilesURL = "https://files.example.com"
)

// Feature serves the API documentation.
type Feature struct {
	cfg *config.Config
	log logger.Logger
}

// New builds the slice.
func New(cfg *config.Config, log logger.Logger) *Feature {
	return &Feature{cfg: cfg, log: log}
}

// MiddlewareSet is empty: the docs are public.
type MiddlewareSet struct{}

// Register mounts the routes on r, which is expected to be scoped to `/docs`.
func (f *Feature) Register(r chi.Router, _ MiddlewareSet) {
	r.Get("/", f.ServeSwaggerUI)
	r.Get("/swagger.yaml", f.ServeSwaggerYAML)
}

// specVars are the per-deployment values injected into swagger.yaml.
type specVars struct {
	// APIName is the brand shown in the description and contact block. From
	// PUBLIC_API_NAME.
	APIName string
	// APITitle is the document title. Computed, not a variable of its own: an
	// unconfigured deployment renders "API" rather than "API API".
	APITitle string
	// APIBaseURL is the public base URL advertised in `servers[]`. From
	// PUBLIC_API_URL, falling back to the host of the current request.
	APIBaseURL string
	// FilesURL is the CDN prefix used by the certificate example. From
	// PUBLIC_FILES_URL.
	FilesURL string
}

func (f *Feature) ServeSwaggerYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var buf bytes.Buffer
	if err := swaggerSpecTemplate.Execute(&buf, f.specVars(r)); err != nil {
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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vars := f.specVars(r)
	swaggerURL := requestBaseURL(r) + "/docs/swagger.yaml"

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
                url: "` + swaggerURL + `",
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

// specVars resolves this deployment's branding. Every field falls back to a
// neutral placeholder, so the rendered spec is a well-formed document even on a
// half-configured deployment.
func (f *Feature) specVars(r *http.Request) specVars {
	apiName := f.cfg.Public.APIName
	apiTitle := apiName + " API"
	if apiName == "" {
		apiName = defaultAPIName
		apiTitle = defaultAPIName
	}

	// Trailing slash trimmed: the spec's paths already start with "/", and
	// `https://host//api/v1` is an ugly (and in some clients, broken) URL.
	baseURL := strings.TrimRight(f.cfg.Public.APIURL, "/")
	if baseURL == "" {
		baseURL = requestBaseURL(r)
	}

	filesURL := strings.TrimRight(f.cfg.Public.FilesURL, "/")
	if filesURL == "" {
		filesURL = defaultFilesURL
	}

	return specVars{
		APIName:    apiName,
		APITitle:   apiTitle,
		APIBaseURL: baseURL,
		FilesURL:   filesURL,
	}
}

// requestBaseURL rebuilds the public scheme+host the caller used, honouring the
// proxy header since the service always runs behind one in production.
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
