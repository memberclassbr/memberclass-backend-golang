package docs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memberclass-backend-golang/internal/platform/config"
	"gopkg.in/yaml.v3"
)

// discardLogger keeps the slice's error path out of the test output.
type discardLogger struct{}

func (discardLogger) Debug(string, ...any) {}
func (discardLogger) Info(string, ...any)  {}
func (discardLogger) Warn(string, ...any)  {}
func (discardLogger) Error(string, ...any) {}

// parsedSpec covers the fields this slice templates. Unmarshalling into it is
// also the assertion that the rendered output is still valid YAML.
type parsedSpec struct {
	Info struct {
		Title       string `yaml:"title"`
		Description string `yaml:"description"`
		Contact     struct {
			Name string `yaml:"name"`
		} `yaml:"contact"`
	} `yaml:"info"`
	Servers []struct {
		URL string `yaml:"url"`
	} `yaml:"servers"`
}

func featureWith(public config.Public) *Feature {
	return New(&config.Config{Public: public}, discardLogger{})
}

func renderSpec(t *testing.T, f *Feature, req *http.Request) (string, parsedSpec) {
	t.Helper()

	rec := httptest.NewRecorder()
	f.ServeSwaggerYAML(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()

	var spec parsedSpec
	if err := yaml.Unmarshal([]byte(body), &spec); err != nil {
		t.Fatalf("rendered spec is not valid YAML: %v", err)
	}

	return body, spec
}

func TestServeSwaggerYAMLUsesDeploymentBranding(t *testing.T) {
	f := featureWith(config.Public{
		APIName:  "Ephra",
		APIURL:   "https://api.ephra.com.br/",
		FilesURL: "https://storage.filesephra.com",
	})

	body, spec := renderSpec(t, f, httptest.NewRequest(http.MethodGet, "/docs/swagger.yaml", nil))

	if spec.Info.Title != "Ephra API" {
		t.Errorf("title = %q, want %q", spec.Info.Title, "Ephra API")
	}
	if spec.Info.Contact.Name != "Ephra Team" {
		t.Errorf("contact.name = %q, want %q", spec.Info.Contact.Name, "Ephra Team")
	}
	if !strings.Contains(spec.Info.Description, "sistema Ephra") {
		t.Errorf("description does not name the deployment: %q", spec.Info.Description)
	}
	// Trailing slash trimmed: the spec's paths already start with "/".
	if len(spec.Servers) == 0 || spec.Servers[0].URL != "https://api.ephra.com.br" {
		t.Errorf("servers = %+v, want first url https://api.ephra.com.br", spec.Servers)
	}
	if !strings.Contains(body, "https://storage.filesephra.com/certificate-....pdf") {
		t.Error("certificate example does not use PUBLIC_FILES_URL")
	}
}

// The regression guard: no deployment may ever serve another one's brand.
func TestServeSwaggerYAMLDoesNotLeakAnotherBrand(t *testing.T) {
	f := featureWith(config.Public{APIName: "Ephra", APIURL: "https://api.ephra.com.br"})

	body, _ := renderSpec(t, f, httptest.NewRequest(http.MethodGet, "/docs/swagger.yaml", nil))

	for _, leaked := range []string{"MemberClass", "memberclass"} {
		if strings.Contains(body, leaked) {
			t.Errorf("rendered spec still contains %q", leaked)
		}
	}
}

func TestServeSwaggerYAMLFallsBackToRequestHost(t *testing.T) {
	f := featureWith(config.Public{APIName: "Ephra"})

	req := httptest.NewRequest(http.MethodGet, "/docs/swagger.yaml", nil)
	req.Host = "api.ephra.com.br"
	req.Header.Set("X-Forwarded-Proto", "https")

	_, spec := renderSpec(t, f, req)

	if len(spec.Servers) == 0 || spec.Servers[0].URL != "https://api.ephra.com.br" {
		t.Errorf("servers = %+v, want first url https://api.ephra.com.br", spec.Servers)
	}
}

func TestServeSwaggerYAMLRendersNeutrallyWhenUnconfigured(t *testing.T) {
	f := featureWith(config.Public{})

	body, spec := renderSpec(t, f, httptest.NewRequest(http.MethodGet, "/docs/swagger.yaml", nil))

	if spec.Info.Title != "API" {
		t.Errorf("title = %q, want %q", spec.Info.Title, "API")
	}
	if spec.Info.Contact.Name != "API Team" {
		t.Errorf("contact.name = %q, want %q", spec.Info.Contact.Name, "API Team")
	}
	if !strings.Contains(body, "https://files.example.com/certificate-....pdf") {
		t.Error("certificate example does not fall back to a neutral host")
	}
	if strings.Contains(body, "MemberClass") {
		t.Error("unconfigured deployment rendered a brand")
	}
}

func TestServeSwaggerYAMLLeavesNoPlaceholderUnresolved(t *testing.T) {
	f := featureWith(config.Public{})

	body, _ := renderSpec(t, f, httptest.NewRequest(http.MethodGet, "/docs/swagger.yaml", nil))

	if strings.Contains(body, "{{") {
		t.Error("rendered spec still contains a template placeholder")
	}
}

func TestServeSwaggerUIUsesDeploymentBranding(t *testing.T) {
	f := featureWith(config.Public{APIName: "Ephra"})

	req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	req.Host = "api.ephra.com.br"
	req.Header.Set("X-Forwarded-Proto", "https")

	rec := httptest.NewRecorder()
	f.ServeSwaggerUI(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()

	if !strings.Contains(body, "<title>Ephra API - Swagger UI</title>") {
		t.Error("page title does not name the deployment")
	}
	if !strings.Contains(body, `url: "https://api.ephra.com.br/docs/swagger.yaml"`) {
		t.Error("Swagger UI does not point at this host's spec")
	}
	if strings.Contains(body, "MemberClass") {
		t.Error("page still contains a hardcoded brand")
	}
}
