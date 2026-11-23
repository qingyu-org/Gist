package settings

import (
	"strings"

	"github.com/qingyu-org/Gist/internal/config"
)

// The third-party sign-in providers the admin panel can configure.
//
// Every provider gets an enable switch followed by the fields listed in its
// spec. i18n keys are derived from the provider and field names, so adding a
// provider means adding one spec here plus its strings in the locale files:
//
//	settings.oauth.<provider>_enabled(_help)
//	settings.oauth.<provider>_<field>_help   (label is the same without _help)
//	settings.oauth.tab_<provider>            (tab label in the admin panel)
type oauthProviderSpec struct {
	Key    string
	Fields []oauthFieldSpec
}

type oauthFieldSpec struct {
	Key         string // key suffix, also the i18n suffix with "-" replaced by "_"
	Kind        Kind
	Default     string
	Placeholder string
	Max         int
}

var oauthProviders = []oauthProviderSpec{
	{Key: "github", Fields: []oauthFieldSpec{
		{Key: "name", Kind: KindString, Default: "GitHub", Placeholder: "GitHub", Max: 80},
		{Key: "client-key", Kind: KindString, Placeholder: "Iv1.0123456789abcdef", Max: 200},
		{Key: "secret", Kind: KindSecret, Max: 300},
	}},
	{Key: "google", Fields: []oauthFieldSpec{
		{Key: "name", Kind: KindString, Default: "Google", Placeholder: "Google", Max: 80},
		{Key: "client-key", Kind: KindString, Placeholder: "123456789012-abc.apps.googleusercontent.com", Max: 300},
		{Key: "secret", Kind: KindSecret, Max: 300},
	}},
	{Key: "gitlab", Fields: []oauthFieldSpec{
		{Key: "name", Kind: KindString, Default: "GitLab", Placeholder: "GitLab", Max: 80},
		{Key: "client-key", Kind: KindString, Placeholder: "0123456789abcdef0123456789abcdef", Max: 200},
		{Key: "secret", Kind: KindSecret, Max: 300},
		{Key: "url", Kind: KindURL, Default: DefaultGitLabURL, Placeholder: DefaultGitLabURL, Max: 300},
	}},
	{Key: "gitea", Fields: []oauthFieldSpec{
		{Key: "name", Kind: KindString, Default: "Gitea", Placeholder: "Gitea", Max: 80},
		{Key: "client-key", Kind: KindString, Placeholder: "client-id", Max: 200},
		{Key: "secret", Kind: KindSecret, Max: 300},
		{Key: "url", Kind: KindURL, Default: "https://gitea.com", Placeholder: "https://gitea.com", Max: 300},
	}},
	{Key: "oidc", Fields: []oauthFieldSpec{
		{Key: "name", Kind: KindString, Default: "Single sign-on", Placeholder: "Corporate SSO", Max: 80},
		{Key: "client-key", Kind: KindString, Placeholder: "client-id", Max: 200},
		{Key: "secret", Kind: KindSecret, Max: 300},
		{Key: "discovery-url", Kind: KindURL, Placeholder: "https://id.example.com/.well-known/openid-configuration", Max: 500},
		{Key: "group-claim", Kind: KindString, Placeholder: "groups", Max: 100},
		{Key: "admin-group", Kind: KindString, Placeholder: "qingyugist-admins", Max: 100},
	}},
	{Key: "oauth2", Fields: []oauthFieldSpec{
		{Key: "name", Kind: KindString, Default: "OAuth 2.0", Placeholder: "OAuth 2.0", Max: 80},
		{Key: "client-key", Kind: KindString, Placeholder: "client-id", Max: 200},
		{Key: "secret", Kind: KindSecret, Max: 300},
		{Key: "auth-url", Kind: KindURL, Placeholder: "https://provider.example.com/oauth/authorize", Max: 500},
		{Key: "token-url", Kind: KindURL, Placeholder: "https://provider.example.com/oauth/token", Max: 500},
		{Key: "userinfo-url", Kind: KindURL, Placeholder: "https://provider.example.com/api/user", Max: 500},
		{Key: "scopes", Kind: KindString, Default: "openid profile email", Placeholder: "openid profile email", Max: 300},
		{Key: "id-field", Kind: KindString, Default: "id", Placeholder: "id", Max: 100},
		{Key: "name-field", Kind: KindString, Default: "username", Placeholder: "username", Max: 100},
		{Key: "email-field", Kind: KindString, Default: "email", Placeholder: "email", Max: 100},
	}},
}

// oauthKey is the database key of one provider field.
func oauthKey(provider, field string) string { return "oauth." + provider + "." + field }

// IsOAuthEnabledKey reports whether key is a provider's enable switch.
func IsOAuthEnabledKey(key string) bool {
	return strings.HasSuffix(key, ".enabled") && strings.HasPrefix(key, "oauth.")
}

// oauthI18n turns a field key into its i18n suffix ("client-key" → "client_key").
func oauthI18n(provider, field string) string {
	return "settings.oauth." + provider + "_" + strings.ReplaceAll(field, "-", "_")
}

// oauthDefinitions builds the category's definitions: one switch and the
// provider's fields, so the panel can render every provider the same way.
func oauthDefinitions() []Definition {
	defs := make([]Definition, 0, 64)
	for _, p := range oauthProviders {
		defs = append(defs, Definition{
			Key:      oauthKey(p.Key, "enabled"),
			Kind:     KindBool,
			Default:  "0",
			Category: CategoryOAuth,
			Provider: p.Key,
			Label:    oauthI18n(p.Key, "enabled"),
			Help:     oauthI18n(p.Key, "enabled") + "_help",
		})
		for _, f := range p.Fields {
			defs = append(defs, Definition{
				Key:         oauthKey(p.Key, f.Key),
				Kind:        f.Kind,
				Default:     f.Default,
				Category:    CategoryOAuth,
				Provider:    p.Key,
				Label:       oauthI18n(p.Key, f.Key),
				Help:        oauthI18n(p.Key, f.Key) + "_help",
				Placeholder: f.Placeholder,
				Max:         f.Max,
			})
		}
	}
	return defs
}

// The two tables below map "<provider>.<field>" to the config field it mirrors.
// They are the single place where a new provider field has to be wired up.
var oauthBoolFields = map[string]func(c *config.Config) *bool{
	"github.enabled": func(c *config.Config) *bool { return &c.GithubEnabled },
	"google.enabled": func(c *config.Config) *bool { return &c.GoogleEnabled },
	"gitlab.enabled": func(c *config.Config) *bool { return &c.GitlabEnabled },
	"gitea.enabled":  func(c *config.Config) *bool { return &c.GiteaEnabled },
	"oidc.enabled":   func(c *config.Config) *bool { return &c.OIDCEnabled },
	"oauth2.enabled": func(c *config.Config) *bool { return &c.OAuth2Enabled },
}

var oauthStringFields = map[string]func(c *config.Config) *string{
	"github.name":       func(c *config.Config) *string { return &c.GithubName },
	"github.client-key": func(c *config.Config) *string { return &c.GithubClientKey },
	"github.secret":     func(c *config.Config) *string { return &c.GithubSecret },

	"google.name":       func(c *config.Config) *string { return &c.GoogleName },
	"google.client-key": func(c *config.Config) *string { return &c.GoogleClientKey },
	"google.secret":     func(c *config.Config) *string { return &c.GoogleSecret },

	"gitlab.name":       func(c *config.Config) *string { return &c.GitlabName },
	"gitlab.client-key": func(c *config.Config) *string { return &c.GitlabClientKey },
	"gitlab.secret":     func(c *config.Config) *string { return &c.GitlabSecret },
	"gitlab.url":        func(c *config.Config) *string { return &c.GitlabUrl },

	"gitea.name":       func(c *config.Config) *string { return &c.GiteaName },
	"gitea.client-key": func(c *config.Config) *string { return &c.GiteaClientKey },
	"gitea.secret":     func(c *config.Config) *string { return &c.GiteaSecret },
	"gitea.url":        func(c *config.Config) *string { return &c.GiteaUrl },

	"oidc.name":          func(c *config.Config) *string { return &c.OIDCProviderName },
	"oidc.client-key":    func(c *config.Config) *string { return &c.OIDCClientKey },
	"oidc.secret":        func(c *config.Config) *string { return &c.OIDCSecret },
	"oidc.discovery-url": func(c *config.Config) *string { return &c.OIDCDiscoveryUrl },
	"oidc.group-claim":   func(c *config.Config) *string { return &c.OIDCGroupClaimName },
	"oidc.admin-group":   func(c *config.Config) *string { return &c.OIDCAdminGroup },

	"oauth2.name":         func(c *config.Config) *string { return &c.OAuth2Name },
	"oauth2.client-key":   func(c *config.Config) *string { return &c.OAuth2ClientKey },
	"oauth2.secret":       func(c *config.Config) *string { return &c.OAuth2Secret },
	"oauth2.auth-url":     func(c *config.Config) *string { return &c.OAuth2AuthUrl },
	"oauth2.token-url":    func(c *config.Config) *string { return &c.OAuth2TokenUrl },
	"oauth2.userinfo-url": func(c *config.Config) *string { return &c.OAuth2UserinfoUrl },
	"oauth2.scopes":       func(c *config.Config) *string { return &c.OAuth2Scopes },
	"oauth2.id-field":     func(c *config.Config) *string { return &c.OAuth2IdField },
	"oauth2.name-field":   func(c *config.Config) *string { return &c.OAuth2NameField },
	"oauth2.email-field":  func(c *config.Config) *string { return &c.OAuth2EmailField },
}

// oauthConfigValue reads the config value backing a definition, used to seed
// the database with whatever the environment provided.
func oauthConfigValue(c *config.Config, provider, field string) (string, bool) {
	if field == "enabled" {
		setter, ok := oauthBoolFields[provider+".enabled"]
		if !ok {
			return "", false
		}
		if *setter(c) {
			return "1", true
		}
		return "0", true
	}

	setter, ok := oauthStringFields[provider+"."+field]
	if !ok {
		return "", false
	}
	return *setter(c), true
}

// applyOAuthSettings overlays the stored third-party values onto the config
// snapshot. Stored values win, except when they are empty: an empty field means
// "unset", so the boot value (environment or compiled default) stays in place.
func applyOAuthSettings(c *config.Config, v map[string]string) {
	for _, p := range oauthProviders {
		if raw, ok := v[oauthKey(p.Key, "enabled")]; ok {
			if setter, ok := oauthBoolFields[p.Key+".enabled"]; ok {
				*setter(c) = raw == "1"
			}
		}
		for _, f := range p.Fields {
			setter, ok := oauthStringFields[p.Key+"."+f.Key]
			if !ok {
				continue
			}
			raw, ok := v[oauthKey(p.Key, f.Key)]
			if !ok {
				continue
			}
			value := strings.TrimSpace(raw)
			if value == "" {
				continue
			}
			if f.Kind == KindURL {
				value = strings.TrimRight(value, "/")
			}
			*setter(c) = value
		}
	}
}
