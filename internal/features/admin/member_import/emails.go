package member_import

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/memberclass-backend-golang/internal/infrastructure/adapters/external_services/resend"
)

// Template types in the TenantEmailTemplate table — match the Next.js constants.
const (
	tmplAccessLink     = "access_link"
	tmplAccessDelivery = "access_delivery"
)

// emailTemplateOverride mirrors the subset of TenantEmailTemplate columns we
// care about. Empty string = "use the default".
type emailTemplateOverride struct {
	Subject    string
	Preview    string
	Title      string
	Greeting   string
	BodyText   string
	ButtonText string
	FooterText string
}

// ---------- Batch orchestration ----------

// sendBatchEmails splits a batch's rows into (new users, users with new
// deliveries) and ships each group through a single Resend batch call.
// Mutates `states` to record per-row email outcomes.
func (f *Feature) sendBatchEmails(
	ctx context.Context,
	importID string,
	tenant *tenantRow,
	states []rowState,
	passwordAccount string,
	counters *importCounters,
) {
	var loginIdx, deliveryIdx []int
	for i := range states {
		s := &states[i]
		if s.status == "error" || s.status == "skipped" {
			continue
		}
		if s.magicToken == "" {
			continue
		}
		if s.isNewUser {
			loginIdx = append(loginIdx, i)
		} else {
			deliveryIdx = append(deliveryIdx, i)
		}
	}

	if len(loginIdx) > 0 {
		tmpl, err := f.fetchTemplateOverride(ctx, tenant.ID, tmplAccessLink)
		if err != nil {
			f.log.Error("import.template_fetch_failed",
				"import_id", importID, "type", tmplAccessLink, "error", err.Error())
		}
		f.sendGroup(ctx, importID, tenant, states, loginIdx, "login", passwordAccount, tmpl, counters)
	}

	if len(deliveryIdx) > 0 {
		tmpl, err := f.fetchTemplateOverride(ctx, tenant.ID, tmplAccessDelivery)
		if err != nil {
			f.log.Error("import.template_fetch_failed",
				"import_id", importID, "type", tmplAccessDelivery, "error", err.Error())
		}
		f.sendGroup(ctx, importID, tenant, states, deliveryIdx, "delivery", "", tmpl, counters)
	}
}

// sendGroup builds N Resend.Email items from the selected row indexes and
// ships them as a single batch.
func (f *Feature) sendGroup(
	ctx context.Context,
	importID string,
	tenant *tenantRow,
	states []rowState,
	idx []int,
	kind string,
	passwordAccount string,
	tmpl *emailTemplateOverride,
	counters *importCounters,
) {
	// Single source of truth for both the From-host (always memberclass.com.br)
	// AND the root under which tenant subdomains are built. PUBLIC_ROOT_DOMAIN
	// is intentionally NOT used here — that env is the backend's own port
	// (localhost:8181 in dev) and would produce broken magic links.
	publicRoot := normalizeEmailDomain(firstNonEmpty(
		os.Getenv("PUBLIC_DOMAIN_URL"),
		os.Getenv("NEXT_PUBLIC_DOMAIN_URL"),
	))
	if publicRoot == "" {
		f.log.Error("import.public_domain_missing",
			"import_id", importID,
			"hint", "set PUBLIC_DOMAIN_URL (or NEXT_PUBLIC_DOMAIN_URL) to a bare host like memberclass.com.br",
		)
		for _, i := range idx {
			states[i].emailSent = sql.NullString{Valid: true, String: kind}
			states[i].emailStatus = sql.NullString{Valid: true, String: "failed"}
			counters.emailsFailed++
		}
		return
	}

	tenantDom := tenantDomain(tenant, publicRoot)
	proto := pickProtocol(tenantDom)

	transLang := ""
	if tenant.Language.Valid {
		transLang = tenant.Language.String
	}
	i18n := translationsFor(transLang)

	fromAddress := fmt.Sprintf("%s <naoresponder@%s>", sanitizeEmailName(tenant.Name), publicRoot)

	emails := make([]resend.Email, 0, len(idx))
	for _, i := range idx {
		s := &states[i]
		link := buildMagicLink(proto, tenantDom, s.shortCode, s.magicToken, s.input.Email)
		subject, html := renderEmail(kind, s, tenant, link, passwordAccount, tmpl, i18n)
		emails = append(emails, resend.Email{
			From:    fromAddress,
			To:      []string{strings.ToLower(s.input.Email)},
			Subject: subject,
			HTML:    html,
		})
	}

	_, err := f.resend.SendBatch(ctx, emails)
	if err != nil {
		f.log.Error("import.email_batch_failed",
			"import_id", importID, "kind", kind, "error", err.Error())
		for _, i := range idx {
			states[i].emailSent = sql.NullString{Valid: true, String: kind}
			states[i].emailStatus = sql.NullString{Valid: true, String: "failed"}
			counters.emailsFailed++
		}
		return
	}

	for _, i := range idx {
		states[i].emailSent = sql.NullString{Valid: true, String: kind}
		states[i].emailStatus = sql.NullString{Valid: true, String: "sent"}
	}
	if kind == "login" {
		counters.loginEmailsSent += len(idx)
	} else {
		counters.deliveryEmailsSent += len(idx)
	}
}

// ---------- Link builder ----------

// buildMagicLink matches the Next.js buildMagicLink(): prefer shortCode when
// available, otherwise fall back to the raw token + email.
func buildMagicLink(proto, domain, shortCode, token, email string) string {
	v := url.Values{}
	if shortCode != "" {
		v.Set("code", shortCode)
	} else {
		if token != "" {
			v.Set("token", token)
		}
		if email != "" {
			v.Set("email", strings.ToLower(email))
		}
	}
	v.Set("isReset", "false")
	return fmt.Sprintf("%s://%s/login?%s", proto, domain, v.Encode())
}

// ---------- Template override lookup ----------

// fetchTemplateOverride reads the tenant's per-type customization from
// TenantEmailTemplate. Returns nil (and nil error) if no row exists — the
// caller then uses the built-in default.
func (f *Feature) fetchTemplateOverride(ctx context.Context, tenantID, tmplType string) (*emailTemplateOverride, error) {
	const q = `
		SELECT COALESCE(subject, ''), COALESCE(preview, ''), COALESCE(title, ''),
		       COALESCE(greeting, ''), COALESCE("bodyText", ''),
		       COALESCE("buttonText", ''), COALESCE("footerText", '')
		FROM "TenantEmailTemplate"
		WHERE "tenantId" = $1 AND type = $2
		LIMIT 1
	`
	o := &emailTemplateOverride{}
	err := f.db.QueryRowContext(ctx, q, tenantID, tmplType).Scan(
		&o.Subject, &o.Preview, &o.Title, &o.Greeting, &o.BodyText, &o.ButtonText, &o.FooterText,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return o, nil
}

// ---------- Rendering ----------

// emailData is the fully-resolved payload the HTML template consumes. Every
// *derived* value is baked here so the template is purely presentational and
// has no branching logic beyond what the React version does (logo guard,
// credentials guard, multi-line body/footer).
type emailData struct {
	// Tenant-scoped
	AreaName        string
	AreaEmail       string
	Logo            string // absolute URL (resolved)
	MainColor       string // accent: button bg, heading color
	BackgroundColor string
	TextColor       string
	// Derived palette (computed once from BackgroundColor)
	MutedColor string // captions, hints
	CodeBg     string // <code> box fill
	CodeBorder string
	HrColor    string
	// User-scoped
	Name     string
	Email    string
	Password string // empty = credentials block hidden
	// Flow-scoped
	Link      string
	ResetLink string // delivery only
	// Copy (post-replacement, ready to render)
	Subject    string
	Preview    string
	Title      string
	Greeting   string
	BodyLines  []string // multi-line body split on "\n"
	ButtonText string
	// Fixed labels from i18n
	CopyPasteHint       string
	CredentialsHint     string
	EmailLabel          string
	PasswordLabel       string
	FooterLines         []string // multi-line footer split on "\n"
	ResetPasswordHint   string
	ResetPasswordButton string
}

// renderEmail picks the right template, resolves i18n + override + variable
// substitution, and returns (subject, html).
func renderEmail(
	kind string,
	s *rowState,
	tenant *tenantRow,
	link, passwordAccount string,
	override *emailTemplateOverride,
	i18n emailTranslations,
) (string, string) {
	name := nameOrEmail(s.name, s.input.Email)
	areaName := tenant.Name
	areaEmail := stringOr(tenant.EmailContact, "")

	// Substitute {{name}}, {{areaName}}, {{areaEmail}}, {{link}} on every
	// user-facing string — both i18n defaults and tenant overrides. Same
	// placeholder grammar as the Next.js side.
	replacer := strings.NewReplacer(
		"{{name}}", name,
		"{{areaName}}", areaName,
		"{{areaEmail}}", areaEmail,
		"{{link}}", link,
	)

	pick := func(over, fallback string) string {
		if strings.TrimSpace(over) != "" {
			return replacer.Replace(over)
		}
		return replacer.Replace(fallback)
	}

	if override == nil {
		override = &emailTemplateOverride{}
	}

	bg := stringOr(tenant.BackgroundColor, "#0a0a0a")
	tx := stringOr(tenant.TextColor, "#fafafa")
	palette := derivePalette(bg)

	data := emailData{
		AreaName:            areaName,
		AreaEmail:           areaEmail,
		Logo:                resolveLogoURL(stringOr(tenant.Logo, "")),
		MainColor:           stringOr(tenant.MainColor, "#D946EF"),
		BackgroundColor:     bg,
		TextColor:           tx,
		MutedColor:          palette.muted,
		CodeBg:              palette.codeBg,
		CodeBorder:          palette.codeBorder,
		HrColor:             palette.hr,
		Name:                name,
		Email:               strings.ToLower(s.input.Email),
		Password:            passwordAccount,
		Link:                link,
		CopyPasteHint:       i18n.CopyPasteHint,
		CredentialsHint:     i18n.CredentialsHint,
		EmailLabel:          i18n.EmailLabel,
		PasswordLabel:       i18n.PasswordLabel,
		ResetPasswordHint:   i18n.ResetPasswordHint,
		ResetPasswordButton: i18n.ResetPasswordButton,
	}

	var bodyTmpl *template.Template
	var subjectSource, titleSource, greetingSource, bodySource, buttonSource, previewSource string

	if kind == "login" {
		bodyTmpl = loginBodyTmpl
		subjectSource = i18n.LoginSubject
		titleSource = i18n.LoginTitle
		greetingSource = i18n.LoginGreeting
		bodySource = i18n.LoginBody
		buttonSource = i18n.LoginButton
		previewSource = i18n.LoginSubject
	} else {
		bodyTmpl = deliveryBodyTmpl
		subjectSource = i18n.DeliverySubject
		titleSource = i18n.DeliveryTitle
		greetingSource = i18n.DeliveryGreeting
		bodySource = i18n.DeliveryBody
		buttonSource = i18n.DeliveryButton
		previewSource = i18n.DeliverySubject

		if strings.Contains(link, "?") {
			data.ResetLink = link + "&isReset=true"
		} else {
			data.ResetLink = link + "?isReset=true"
		}
	}

	data.Subject = pick(override.Subject, subjectSource)
	data.Preview = pick(override.Preview, previewSource)
	data.Title = pick(override.Title, titleSource)
	data.Greeting = pick(override.Greeting, greetingSource)
	data.BodyLines = strings.Split(pick(override.BodyText, bodySource), "\n")
	data.ButtonText = pick(override.ButtonText, buttonSource)
	data.FooterLines = strings.Split(pick(override.FooterText, i18n.FooterContact), "\n")

	var buf bytes.Buffer
	_ = bodyTmpl.Execute(&buf, data)
	return data.Subject, buf.String()
}

func stringOr(v sql.NullString, fallback string) string {
	if v.Valid && v.String != "" {
		return v.String
	}
	return fallback
}

func nameOrEmail(name, email string) string {
	if name != "" {
		return name
	}
	return strings.ToLower(email)
}

// resolveLogoURL mirrors the Next.js logic:
//
//	logo.startsWith("https://") ? logo : PUBLIC_FILES_URL + logo
//
// With PUBLIC_FILES_URL (or NEXT_PUBLIC_FILES_URL) as the CDN/S3 prefix
// where relative paths live. Empty logo stays empty (the template then
// hides the <img>).
func resolveLogoURL(logo string) string {
	if logo == "" {
		return ""
	}
	lower := strings.ToLower(logo)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return logo
	}
	prefix := firstNonEmpty(os.Getenv("PUBLIC_FILES_URL"), os.Getenv("NEXT_PUBLIC_FILES_URL"))
	if prefix == "" {
		return logo
	}
	prefix = strings.TrimRight(prefix, "/")
	if strings.HasPrefix(logo, "/") {
		return prefix + logo
	}
	return prefix + "/" + logo
}

// ---------- Derived palette ----------

type palette struct {
	muted      string
	codeBg     string
	codeBorder string
	hr         string
}

var darkPalette = palette{
	muted:      "#a3a3a3",
	codeBg:     "#1a1a1a",
	codeBorder: "#2a2a2a",
	hr:         "#262626",
}

var lightPalette = palette{
	muted:      "#737373",
	codeBg:     "#f4f4f5",
	codeBorder: "#e4e4e7",
	hr:         "#e5e5e5",
}

// derivePalette picks dark- or light-theme helper colors based on the
// perceived brightness of the background. Keeps the email looking balanced
// whether the tenant uses a dark (default) or light brand background.
func derivePalette(backgroundHex string) palette {
	if isDarkBg(backgroundHex) {
		return darkPalette
	}
	return lightPalette
}

// isDarkBg returns true when the ITU-R BT.601 perceived brightness of the
// hex color is below 128/255. Malformed inputs default to "dark" since the
// memberclass brand is dark-first.
func isDarkBg(hex string) bool {
	hex = strings.TrimSpace(strings.TrimPrefix(hex, "#"))
	if len(hex) == 3 {
		// #abc → #aabbcc
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return true
	}
	r, err1 := strconv.ParseInt(hex[0:2], 16, 64)
	g, err2 := strconv.ParseInt(hex[2:4], 16, 64)
	b, err3 := strconv.ParseInt(hex[4:6], 16, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return true
	}
	brightness := (299*r + 587*g + 114*b) / 1000
	return brightness < 128
}

// ---------- Text helpers ----------

// firstNonEmpty returns the first trimmed-non-empty element, or "".
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// normalizeEmailDomain turns a potentially URL-shaped input into a bare host
// suitable for an email address (RFC 5321 forbids port / path / scheme in
// the domain part).
func normalizeEmailDomain(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, scheme := range []string{"https://", "http://"} {
		if strings.HasPrefix(strings.ToLower(s), scheme) {
			s = s[len(scheme):]
			break
		}
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// sanitizeEmailName strips characters that would break the address-list
// grammar of RFC 5322 when inlined into `Name <email>` without quoting.
func sanitizeEmailName(name string) string {
	name = strings.TrimSpace(name)
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', '"', '\r', '\n':
			return -1
		}
		return r
	}, name)
	if cleaned == "" {
		return "no-reply"
	}
	return cleaned
}

// ---------- Built-in HTML templates ----------
//
// Table-based layout for widest email-client compatibility (Outlook on
// Windows still uses MSHTML, which chokes on flexbox). Every style is
// inline — gmail strips <style> blocks.

var loginBodyTmpl = template.Must(template.New("login-body").Parse(loginHTML))
var deliveryBodyTmpl = template.Must(template.New("delivery-body").Parse(deliveryHTML))

const loginHTML = `<!doctype html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="x-apple-disable-message-reformatting">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>{{.AreaName}}</title>
</head>
<body style="margin:0;padding:0;background:{{.BackgroundColor}};color:{{.TextColor}};font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Oxygen,Ubuntu,sans-serif;-webkit-font-smoothing:antialiased;mso-line-height-rule:exactly;">
  <div style="display:none;max-height:0;overflow:hidden;opacity:0;">{{.Preview}}</div>
  <table role="presentation" cellspacing="0" cellpadding="0" border="0" width="100%" style="background:{{.BackgroundColor}};">
    <tr><td align="center" style="padding:40px 16px;">
      <table role="presentation" cellspacing="0" cellpadding="0" border="0" width="100%" style="max-width:560px;">
        {{if .Logo}}
        <tr><td align="center" style="padding:0 0 32px;">
          <img src="{{.Logo}}" alt="{{.AreaName}}" height="56" style="max-height:56px;border:0;outline:none;text-decoration:none;display:inline-block;">
        </td></tr>
        {{end}}

        <tr><td style="padding:0 0 28px;">
          <h1 style="color:{{.MainColor}};font-size:22px;font-weight:700;line-height:1.3;margin:0;">{{.Title}}</h1>
        </td></tr>

        <tr><td style="padding:0 0 24px;">
          <a href="{{.Link}}" style="display:block;background:{{.MainColor}};color:#ffffff;text-decoration:none;text-align:center;padding:14px 20px;border-radius:8px;font-weight:600;font-size:14px;">{{.ButtonText}}</a>
        </td></tr>

        <tr><td style="padding:0 0 8px;">
          <p style="color:{{.MutedColor}};font-size:14px;margin:0;line-height:1.5;">{{.CopyPasteHint}}</p>
        </td></tr>
        <tr><td style="padding:0 0 24px;">
          <div style="background:{{.CodeBg}};border:1px solid {{.CodeBorder}};border-radius:8px;padding:12px 14px;font-family:ui-monospace,'SF Mono',Menlo,Consolas,monospace;font-size:13px;color:{{.TextColor}};line-height:1.5;word-break:break-all;"><a href="{{.Link}}" style="color:{{.TextColor}};text-decoration:underline;">{{.Link}}</a></div>
        </td></tr>

        <tr><td style="padding:0 0 8px;">
          <p style="color:{{.TextColor}};font-size:14px;margin:0;line-height:1.5;">{{.Greeting}}</p>
        </td></tr>

        {{range .BodyLines}}
        <tr><td style="padding:0 0 8px;">
          <p style="color:{{$.TextColor}};font-size:14px;margin:0;line-height:1.5;">{{.}}</p>
        </td></tr>
        {{end}}

        {{if .Password}}
        <tr><td style="padding:16px 0 8px;">
          <p style="color:{{.TextColor}};font-size:14px;margin:0;line-height:1.5;">{{.CredentialsHint}}</p>
        </td></tr>
        <tr><td style="padding:8px 0 4px;">
          <p style="color:{{.TextColor}};font-size:14px;font-weight:600;margin:0;">{{.EmailLabel}}</p>
        </td></tr>
        <tr><td style="padding:0 0 12px;">
          <div style="background:{{.CodeBg}};border:1px solid {{.CodeBorder}};border-radius:8px;padding:14px 16px;font-family:ui-monospace,'SF Mono',Menlo,Consolas,monospace;font-size:15px;color:{{.TextColor}};word-break:break-all;">{{.Email}}</div>
        </td></tr>
        <tr><td style="padding:4px 0 4px;">
          <p style="color:{{.TextColor}};font-size:14px;font-weight:600;margin:0;">{{.PasswordLabel}}</p>
        </td></tr>
        <tr><td style="padding:0 0 20px;">
          <div style="background:{{.CodeBg}};border:1px solid {{.CodeBorder}};border-radius:8px;padding:14px 16px;font-family:ui-monospace,'SF Mono',Menlo,Consolas,monospace;font-size:15px;color:{{.TextColor}};word-break:break-all;">{{.Password}}</div>
        </td></tr>
        {{end}}

        {{range .FooterLines}}
        <tr><td style="padding:0 0 4px;">
          <p style="color:{{$.MutedColor}};font-size:12px;margin:0;line-height:1.5;">{{.}}</p>
        </td></tr>
        {{end}}

        <tr><td style="padding:24px 0 0;">
          <hr style="border:none;border-top:1px solid {{.HrColor}};margin:0 0 12px;">
          <p style="color:{{.MutedColor}};font-size:12px;margin:0;text-align:center;">{{.AreaName}}</p>
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`

const deliveryHTML = `<!doctype html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="x-apple-disable-message-reformatting">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>{{.AreaName}}</title>
</head>
<body style="margin:0;padding:0;background:{{.BackgroundColor}};color:{{.TextColor}};font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Oxygen,Ubuntu,sans-serif;-webkit-font-smoothing:antialiased;mso-line-height-rule:exactly;">
  <div style="display:none;max-height:0;overflow:hidden;opacity:0;">{{.Preview}}</div>
  <table role="presentation" cellspacing="0" cellpadding="0" border="0" width="100%" style="background:{{.BackgroundColor}};">
    <tr><td align="center" style="padding:40px 16px;">
      <table role="presentation" cellspacing="0" cellpadding="0" border="0" width="100%" style="max-width:560px;">
        {{if .Logo}}
        <tr><td align="center" style="padding:0 0 32px;">
          <img src="{{.Logo}}" alt="{{.AreaName}}" height="56" style="max-height:56px;border:0;outline:none;text-decoration:none;display:inline-block;">
        </td></tr>
        {{end}}

        <tr><td style="padding:0 0 28px;">
          <h1 style="color:{{.MainColor}};font-size:22px;font-weight:700;line-height:1.3;margin:0;">{{.Title}}</h1>
        </td></tr>

        <tr><td style="padding:0 0 24px;">
          <a href="{{.Link}}" style="display:block;background:{{.MainColor}};color:#ffffff;text-decoration:none;text-align:center;padding:14px 20px;border-radius:8px;font-weight:600;font-size:14px;">{{.ButtonText}}</a>
        </td></tr>

        <tr><td style="padding:0 0 8px;">
          <p style="color:{{.MutedColor}};font-size:14px;margin:0;line-height:1.5;">{{.CopyPasteHint}}</p>
        </td></tr>
        <tr><td style="padding:0 0 20px;">
          <div style="background:{{.CodeBg}};border:1px solid {{.CodeBorder}};border-radius:8px;padding:12px 14px;font-family:ui-monospace,'SF Mono',Menlo,Consolas,monospace;font-size:13px;color:{{.TextColor}};line-height:1.5;word-break:break-all;"><a href="{{.Link}}" style="color:{{.TextColor}};text-decoration:underline;">{{.Link}}</a></div>
        </td></tr>

        <tr><td style="padding:0 0 8px;">
          <p style="color:{{.TextColor}};font-size:14px;margin:0;line-height:1.5;">{{.Greeting}}</p>
        </td></tr>

        {{range .BodyLines}}
        <tr><td style="padding:0 0 8px;">
          <p style="color:{{$.TextColor}};font-size:14px;margin:0;line-height:1.5;">{{.}}</p>
        </td></tr>
        {{end}}

        <tr><td style="padding:16px 0 8px;">
          <p style="color:{{.MutedColor}};font-size:12px;margin:0;line-height:1.5;">{{.ResetPasswordHint}}</p>
        </td></tr>
        <tr><td style="padding:0 0 20px;">
          <a href="{{.ResetLink}}" style="color:{{.MainColor}};font-size:13px;text-decoration:underline;">{{.ResetPasswordButton}}</a>
        </td></tr>

        {{range .FooterLines}}
        <tr><td style="padding:0 0 4px;">
          <p style="color:{{$.MutedColor}};font-size:12px;margin:0;line-height:1.5;">{{.}}</p>
        </td></tr>
        {{end}}

        <tr><td style="padding:24px 0 0;">
          <hr style="border:none;border-top:1px solid {{.HrColor}};margin:0 0 12px;">
          <p style="color:{{.MutedColor}};font-size:12px;margin:0;text-align:center;">{{.AreaName}}</p>
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`
