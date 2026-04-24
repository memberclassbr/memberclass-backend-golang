package member_import

// emailTranslations holds the per-language strings the email templates fall
// back to when a tenant has NOT customized a given field via
// TenantEmailTemplate. Strings may include the variables {{name}},
// {{areaName}}, {{areaEmail}}, {{link}} — they're substituted by the
// renderer before rendering the HTML template (same rule as the Next.js
// side, so custom templates can use the same placeholders).
type emailTranslations struct {
	// Login (first-access email: magic link + credentials).
	LoginSubject  string
	LoginTitle    string
	LoginGreeting string
	LoginBody     string
	LoginButton   string

	// Delivery (existing user got new deliveries).
	DeliverySubject  string
	DeliveryTitle    string
	DeliveryGreeting string
	DeliveryBody     string
	DeliveryButton   string

	// Shared fragments.
	CopyPasteHint       string // "Se o link acima não funcionar…"
	CredentialsHint     string // "Use os dados abaixo para acessar…"
	EmailLabel          string
	PasswordLabel       string
	FooterContact       string // "Em caso de dúvidas, … {{areaEmail}}"
	ResetPasswordHint   string // "Se você deseja redefinir sua senha, use o link:"
	ResetPasswordButton string
}

// translationsFor returns the pack for the tenant's language, falling back
// to pt-br. Add more languages by extending the switch.
func translationsFor(lang string) emailTranslations {
	switch lang {
	case "en", "en-us", "en-US":
		return english
	default:
		return portuguese
	}
}

var portuguese = emailTranslations{
	LoginSubject:  "🔑 Seu link de acesso para área de membros: {{areaName}}",
	LoginTitle:    "🔑 Seu link de acesso para área de membros: {{areaName}}",
	LoginGreeting: "Olá, {{name}}",
	LoginBody:     "Clique no link para acessar o conteúdo com login direto.",
	LoginButton:   "Acessar área de membros",

	DeliverySubject:  "🔑 Novo conteúdo disponível",
	DeliveryTitle:    "Você tem novos conteúdos",
	DeliveryGreeting: "Olá, {{name}}",
	DeliveryBody:     "Você acabou de ganhar acesso a novos conteúdos. Clique no botão abaixo para entrar.",
	DeliveryButton:   "Acessar área de membros",

	CopyPasteHint:       "Se o link acima não funcionar, copie e cole o link abaixo:",
	CredentialsHint:     "Use os dados abaixo para acessar a área com email e senha.",
	EmailLabel:          "Email:",
	PasswordLabel:       "Senha:",
	FooterContact:       "Em caso de dúvidas, você pode entrar em contato através do e-mail: {{areaEmail}}",
	ResetPasswordHint:   "Se você deseja redefinir sua senha, use o link:",
	ResetPasswordButton: "Redefinir senha",
}

var english = emailTranslations{
	LoginSubject:  "🔑 Your access link to the member area: {{areaName}}",
	LoginTitle:    "🔑 Your access link to the member area: {{areaName}}",
	LoginGreeting: "Hi, {{name}}",
	LoginBody:     "Click the link to open the member area with direct login.",
	LoginButton:   "Open member area",

	DeliverySubject:  "🔑 New content available",
	DeliveryTitle:    "New content is waiting for you",
	DeliveryGreeting: "Hi, {{name}}",
	DeliveryBody:     "You just gained access to new content. Click the button below to sign in.",
	DeliveryButton:   "Open member area",

	CopyPasteHint:       "If the button above doesn't work, copy and paste the link below:",
	CredentialsHint:     "Use the credentials below to sign in with email and password.",
	EmailLabel:          "Email:",
	PasswordLabel:       "Password:",
	FooterContact:       "If you have any questions, contact us at: {{areaEmail}}",
	ResetPasswordHint:   "If you want to reset your password, use the link:",
	ResetPasswordButton: "Reset password",
}
