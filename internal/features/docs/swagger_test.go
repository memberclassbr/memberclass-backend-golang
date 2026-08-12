package docs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// fakeLogger is a local test double — slices don't use generated mocks.
type fakeLogger struct {
	warns []string
}

func (l *fakeLogger) Debug(msg string, args ...any) {}
func (l *fakeLogger) Info(msg string, args ...any)  {}
func (l *fakeLogger) Warn(msg string, args ...any)  { l.warns = append(l.warns, msg) }
func (l *fakeLogger) Error(msg string, args ...any) {}

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

// clearBrandEnv blanks every var the slice reads so each test starts from an
// unconfigured deploy and opts back in explicitly.
func clearBrandEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"PUBLIC_API_NAME",
		"PUBLIC_API_URL",
		"PUBLIC_FILES_URL",
		"NEXT_PUBLIC_FILES_URL",
		"PUBLIC_DOMAIN_URL",
		"NEXT_PUBLIC_DOMAIN_URL",
	} {
		t.Setenv(key, "")
	}
}

func renderSpec(t *testing.T, req *http.Request) (string, parsedSpec) {
	t.Helper()

	rec := httptest.NewRecorder()
	(&Feature{log: &fakeLogger{}}).ServeSwaggerYAML(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	var spec parsedSpec
	require.NoError(t, yaml.Unmarshal([]byte(body), &spec), "rendered spec must be valid YAML")

	return body, spec
}

func TestServeSwaggerYAML_UsesWorkspaceBranding(t *testing.T) {
	clearBrandEnv(t)
	t.Setenv("PUBLIC_API_NAME", "Ephra")
	t.Setenv("PUBLIC_API_URL", "https://api.ephra.com.br/")
	t.Setenv("PUBLIC_FILES_URL", "https://storage.filesephra.com")
	t.Setenv("PUBLIC_DOMAIN_URL", "ephra.com.br")

	body, spec := renderSpec(t, httptest.NewRequest(http.MethodGet, "/docs/swagger.yaml", nil))

	assert.Equal(t, "Ephra API", spec.Info.Title)
	assert.Equal(t, "Ephra Team", spec.Info.Contact.Name)
	assert.Contains(t, spec.Info.Description, "sistema Ephra")

	require.NotEmpty(t, spec.Servers)
	// Trailing slash trimmed: paths in the spec already start with "/".
	assert.Equal(t, "https://api.ephra.com.br", spec.Servers[0].URL)

	assert.Contains(t, body, "https://storage.filesephra.com/certificate-....pdf")
	assert.Contains(t, body, "https://app.ephra.com.br/auth/login?token=abc123")
}

// The regression guard: no workspace may ever see another workspace's brand.
func TestServeSwaggerYAML_NoBrandLeak(t *testing.T) {
	clearBrandEnv(t)
	t.Setenv("PUBLIC_API_NAME", "Ephra")
	t.Setenv("PUBLIC_API_URL", "https://api.ephra.com.br")

	body, _ := renderSpec(t, httptest.NewRequest(http.MethodGet, "/docs/swagger.yaml", nil))

	assert.NotContains(t, body, "MemberClass")
	assert.NotContains(t, body, "memberclass")
}

func TestServeSwaggerYAML_FallsBackToRequestHost(t *testing.T) {
	clearBrandEnv(t)
	t.Setenv("PUBLIC_API_NAME", "Ephra")

	req := httptest.NewRequest(http.MethodGet, "/docs/swagger.yaml", nil)
	req.Host = "api.ephra.com.br"
	req.Header.Set("X-Forwarded-Proto", "https")

	_, spec := renderSpec(t, req)

	require.NotEmpty(t, spec.Servers)
	assert.Equal(t, "https://api.ephra.com.br", spec.Servers[0].URL)
}

func TestServeSwaggerYAML_NeutralDefaultsWhenUnconfigured(t *testing.T) {
	clearBrandEnv(t)

	body, spec := renderSpec(t, httptest.NewRequest(http.MethodGet, "/docs/swagger.yaml", nil))

	assert.Equal(t, "API", spec.Info.Title)
	assert.Equal(t, "API Team", spec.Info.Contact.Name)
	assert.Contains(t, body, "https://files.example.com/certificate-....pdf")
	assert.Contains(t, body, "https://app.example.com/auth/login?token=abc123")
	assert.NotContains(t, body, "MemberClass")
}

func TestServeSwaggerUI_UsesWorkspaceBranding(t *testing.T) {
	clearBrandEnv(t)
	t.Setenv("PUBLIC_API_NAME", "Ephra")

	req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	req.Host = "api.ephra.com.br"
	req.Header.Set("X-Forwarded-Proto", "https")

	rec := httptest.NewRecorder()
	(&Feature{log: &fakeLogger{}}).ServeSwaggerUI(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	assert.Contains(t, body, "<title>Ephra API - Swagger UI</title>")
	assert.Contains(t, body, `url: "https://api.ephra.com.br/docs/swagger.yaml"`)
	assert.NotContains(t, body, "MemberClass")
}

func TestNew_WarnsOnUnconfiguredWorkspace(t *testing.T) {
	clearBrandEnv(t)

	log := &fakeLogger{}
	New(log)

	assert.Equal(t, []string{"docs.api_name_missing", "docs.api_url_missing"}, log.warns)
}

func TestNew_SilentWhenConfigured(t *testing.T) {
	clearBrandEnv(t)
	t.Setenv("PUBLIC_API_NAME", "Ephra")
	t.Setenv("PUBLIC_API_URL", "https://api.ephra.com.br")

	log := &fakeLogger{}
	New(log)

	assert.Empty(t, log.warns)
}

// The spec is a template, so a stray delimiter would only surface at runtime.
// template.Must already panics at init on a syntax error; this pins the
// companion invariant that no placeholder goes unresolved.
func TestSpecTemplate_HasNoUnresolvedPlaceholders(t *testing.T) {
	clearBrandEnv(t)

	body, _ := renderSpec(t, httptest.NewRequest(http.MethodGet, "/docs/swagger.yaml", nil))

	assert.False(t, strings.Contains(body, "{{"), "rendered spec still contains a template placeholder")
}
