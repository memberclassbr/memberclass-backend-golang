// Package docs serves the hand-maintained OpenAPI spec and a Swagger UI for
// it at /docs. The spec is a file next to the binary, copied into the image by
// the Dockerfile.
package docs

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

// Feature serves the API documentation.
type Feature struct {
	swaggerPath string
}

// New builds the slice.
func New() *Feature {
	return &Feature{swaggerPath: "swagger.yaml"}
}

// MiddlewareSet is empty: the docs are public.
type MiddlewareSet struct{}

// Register mounts the routes on r, which is expected to be scoped to `/docs`.
func (f *Feature) Register(r chi.Router, _ MiddlewareSet) {
	r.Get("/", f.ServeSwaggerUI)
	r.Get("/swagger.yaml", f.ServeSwaggerYAML)
}

func (f *Feature) ServeSwaggerYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	paths := []string{
		f.swaggerPath,
		filepath.Join(".", f.swaggerPath),
	}

	workDir, _ := os.Getwd()
	paths = append(paths, filepath.Join(workDir, f.swaggerPath))

	var content []byte
	var err error
	for _, path := range paths {
		content, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		http.Error(w, "Swagger file not found: "+f.swaggerPath, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(content)
}

func (f *Feature) ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		scheme = forwardedProto
	}

	swaggerURL := scheme + "://" + r.Host + "/docs/swagger.yaml"

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>MemberClass API - Swagger UI</title>
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
	w.Write([]byte(html))
}
