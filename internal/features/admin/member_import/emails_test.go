package member_import

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeEmailDomain(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"   ", ""},
		{"memberclass.com.br", "memberclass.com.br"},
		{"  memberclass.com.br  ", "memberclass.com.br"},
		{"https://memberclass.com.br", "memberclass.com.br"},
		{"HTTP://Memberclass.com.br/path", "Memberclass.com.br"},
		{"memberclass.com.br/some/path?x=1", "memberclass.com.br"},
		{"memberclass.com.br#frag", "memberclass.com.br"},
		{"localhost:8181", "localhost"},
		{"https://app.localhost:3000/login", "app.localhost"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeEmailDomain(tc.in))
		})
	}
}

func TestSanitizeEmailName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Acme", "Acme"},
		{"  Acme  ", "Acme"},
		{"", "no-reply"},
		{`"quoted"`, "quoted"},
		{"bad<name>here", "badnamehere"},
		{"line\nbreak", "linebreak"},
		{"São Paulo", "São Paulo"}, // unicode preserved
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, sanitizeEmailName(tc.in))
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	assert.Equal(t, "a", firstNonEmpty("", "a", "b"))
	assert.Equal(t, "b", firstNonEmpty("", "   ", "b"))
	assert.Equal(t, "", firstNonEmpty("", "   "))
}

func TestIsDarkBg(t *testing.T) {
	cases := []struct {
		hex  string
		dark bool
	}{
		{"#000000", true},
		{"#0a0a0a", true},
		{"#111", true}, // #111 → #111111
		{"#ffffff", false},
		{"#f5f5f5", false},
		{"#fff", false},
		{"#555555", true},  // clearly dark side
		{"#cccccc", false}, // clearly light side
		{"not-a-color", true},
		{"", true},
	}
	for _, tc := range cases {
		t.Run(tc.hex, func(t *testing.T) {
			assert.Equal(t, tc.dark, isDarkBg(tc.hex))
		})
	}
}

func TestResolveLogoURL(t *testing.T) {
	// Preserve any value the test env has.
	t.Cleanup(restoreEnv("PUBLIC_FILES_URL"))
	t.Cleanup(restoreEnv("NEXT_PUBLIC_FILES_URL"))

	t.Run("empty stays empty", func(t *testing.T) {
		os.Unsetenv("PUBLIC_FILES_URL")
		os.Unsetenv("NEXT_PUBLIC_FILES_URL")
		assert.Equal(t, "", resolveLogoURL(""))
	})
	t.Run("absolute https URL untouched", func(t *testing.T) {
		os.Setenv("PUBLIC_FILES_URL", "https://files.example.com")
		assert.Equal(t, "https://cdn.example.com/logo.png", resolveLogoURL("https://cdn.example.com/logo.png"))
	})
	t.Run("relative with trailing slash on prefix", func(t *testing.T) {
		os.Setenv("PUBLIC_FILES_URL", "https://files.example.com/")
		assert.Equal(t, "https://files.example.com/logos/acme.png", resolveLogoURL("logos/acme.png"))
	})
	t.Run("relative with leading slash on path", func(t *testing.T) {
		os.Setenv("PUBLIC_FILES_URL", "https://files.example.com")
		assert.Equal(t, "https://files.example.com/logos/acme.png", resolveLogoURL("/logos/acme.png"))
	})
	t.Run("falls back to NEXT_PUBLIC_FILES_URL when PUBLIC_FILES_URL empty", func(t *testing.T) {
		os.Unsetenv("PUBLIC_FILES_URL")
		os.Setenv("NEXT_PUBLIC_FILES_URL", "https://next.example.com")
		assert.Equal(t, "https://next.example.com/logo.png", resolveLogoURL("logo.png"))
	})
	t.Run("no prefix env returns raw logo", func(t *testing.T) {
		os.Unsetenv("PUBLIC_FILES_URL")
		os.Unsetenv("NEXT_PUBLIC_FILES_URL")
		assert.Equal(t, "logos/acme.png", resolveLogoURL("logos/acme.png"))
	})
}

func TestRenderEmail_LoginSubstitutesVariables(t *testing.T) {
	tenant := &tenantRow{
		ID:              "t-1",
		Name:            "Demo3",
		EmailContact:    sql.NullString{Valid: true, String: "contato3@demo.com.br"},
		MainColor:       sql.NullString{Valid: true, String: "#D946EF"},
		BackgroundColor: sql.NullString{Valid: true, String: "#0a0a0a"},
		TextColor:       sql.NullString{Valid: true, String: "#fafafa"},
		Language:        sql.NullString{Valid: true, String: "pt-br"},
	}
	state := &rowState{
		name:  "Tete",
		input: importUserInput{Email: "TETE@example.com"},
	}
	link := "https://demo.memberclass.com.br/login?code=ABC123&isReset=false"
	i18n := translationsFor("pt-br")

	subject, html := renderEmail("login", state, tenant, link, "iL95MxdE", nil, i18n)

	// Subject: `🔑 Seu link ... : {{areaName}}` → {{areaName}} replaced
	assert.Contains(t, subject, "Demo3", "subject substitutes {{areaName}}")
	assert.NotContains(t, subject, "{{areaName}}", "no placeholder leaks")

	// HTML contains the headline, button, link, and credentials block.
	assert.Contains(t, html, "Demo3", "area name rendered")
	assert.Contains(t, html, "Tete", "name substituted in greeting")
	assert.Contains(t, html, "Acessar área de membros", "button text")
	assert.Contains(t, html, "tete@example.com", "email lowercased in credentials")
	assert.Contains(t, html, "iL95MxdE", "password present")
	assert.Contains(t, html, "contato3@demo.com.br", "area email in footer")
	assert.Contains(t, html, "#0a0a0a", "bg color applied")
	assert.Contains(t, html, "#D946EF", "main color applied")
	assert.NotContains(t, html, "{{", "no placeholder leaks in html")
}

func TestRenderEmail_DeliverySkipsCredentials(t *testing.T) {
	tenant := &tenantRow{ID: "t-1", Name: "Demo3"}
	state := &rowState{name: "Tete", input: importUserInput{Email: "tete@example.com"}}
	i18n := translationsFor("pt-br")

	_, html := renderEmail("delivery", state, tenant, "https://x/login?code=A", "", nil, i18n)

	assert.NotContains(t, html, "Use os dados abaixo", "delivery email omits credentials hint")
	assert.Contains(t, html, "Redefinir senha", "delivery shows reset link")
	assert.Contains(t, html, "isReset=true", "reset link carries isReset=true")
}

func TestRenderEmail_OverridePreservesTenantVariables(t *testing.T) {
	tenant := &tenantRow{ID: "t-1", Name: "Acme"}
	state := &rowState{name: "João", input: importUserInput{Email: "joao@example.com"}}
	i18n := translationsFor("pt-br")
	override := &emailTemplateOverride{
		Subject: "Bem-vindo, {{name}} — {{areaName}}",
		Title:   "{{areaName}} te deu acesso!",
	}

	subject, html := renderEmail("login", state, tenant, "https://x", "P@ss", override, i18n)

	assert.Equal(t, "Bem-vindo, João — Acme", subject)
	assert.Contains(t, html, "Acme te deu acesso!")
}

// restoreEnv returns a cleanup closure that resets `key` to its current
// value (or unsets it if it wasn't set). Use with t.Cleanup.
func restoreEnv(key string) func() {
	prev, had := os.LookupEnv(key)
	return func() {
		if had {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	}
}

// Keep the strings import used by the block above (fmt-compliance for go vet).
var _ = strings.Contains
