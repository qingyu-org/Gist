// Package settings implements the instance settings that administrators can
// change at runtime from the admin panel.
//
// The database is the source of truth. Values are cached in memory so hot
// paths (templates, middleware, uploads) never hit the database; Set/SetMany
// validates, persists, refreshes the cache and re-applies the subset of
// settings that mirror boot-time configuration (site name, external URL, ...).
//
// Every key has a definition with a type and, when relevant, an allowed set
// of values. Unknown keys are rejected so a typo in the admin form cannot
// silently create dead settings.
package settings

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/rs/zerolog/log"
)

// Kind is the value type of a setting.
type Kind string

const (
	KindBool   Kind = "bool"
	KindInt    Kind = "int"
	KindString Kind = "string"
	KindText   Kind = "text"
	KindEnum   Kind = "enum"
	KindSecret Kind = "secret"
	KindURL    Kind = "url"
)

// Category groups settings in the admin UI.
type Category string

const (
	CategorySite       Category = "site"
	CategoryAuth       Category = "auth"
	CategoryOAuth      Category = "oauth"
	CategoryLDAP       Category = "ldap"
	CategoryUpload     Category = "upload"
	CategoryComments   Category = "comments"
	CategorySearch     Category = "search"
	CategorySecurity   Category = "security"
	CategoryModeration Category = "moderation"
	CategoryGit        Category = "git"
	CategoryAPI        Category = "api"
)

// Categories lists the admin UI sections in display order.
func Categories() []Category {
	return []Category{
		CategorySite,
		CategoryAuth,
		CategoryOAuth,
		CategoryLDAP,
		CategoryUpload,
		CategoryComments,
		CategorySearch,
		CategorySecurity,
		CategoryModeration,
		CategoryGit,
		CategoryAPI,
	}
}

// Setting keys. Dotted names are used in forms and templates; the legacy
// dash-only keys written by older releases are kept in sync for compatibility.
const (
	KeySiteName         = "site.name"
	KeySiteDescription  = "site.description"
	KeySiteExternalURL  = "site.external-url"
	KeySiteLogo         = "site.logo"
	KeySiteFavicon      = "site.favicon"
	KeySiteCustomCSS    = "site.custom-css"
	KeySiteCustomJS     = "site.custom-js"
	KeySiteRepository   = "site.repository-url"
	KeySiteHomepage     = "site.homepage-url"
	KeySiteEmail        = "site.contact-email"
	KeySiteICP          = "site.icp"
	KeySitePolice       = "site.police-record"
	KeySiteLinks        = "site.links"
	KeySiteAnnouncement = "site.announcement"

	KeyRegistrationMode       = "auth.registration-mode"
	KeyRequireLogin           = "auth.require-login"
	KeyAllowGistsWithoutLogin = "auth.allow-gists-without-login"
	KeyDisableLoginForm       = "auth.disable-login-form"

	// Avatars are looked up by the MD5 hash of the account's email address. The
	// provider is a setting rather than a hardcoded host, so a slow one can be
	// swapped without a rebuild; "none" falls back to the generated identicon
	// and a default image can stand in for accounts without an address.
	KeyAvatarProvider = "avatar.provider"
	KeyAvatarDefault  = "avatar.default"

	// Third-party sign-in. Every provider has an enable switch plus the fields it
	// needs; keys are derived with oauthKey so the surrounding code stays uniform.
	// A provider is offered once its switch is on and its credentials exist.

	KeyLDAPURL          = "ldap.url"
	KeyLDAPBindDN       = "ldap.bind-dn"
	KeyLDAPBindPassword = "ldap.bind-credentials"
	KeyLDAPSearchBase   = "ldap.search-base"
	KeyLDAPSearchFilter = "ldap.search-filter"

	KeyUploadEnabled    = "upload.enabled"
	KeyUploadMaxFileMB  = "upload.max-file-size"
	KeyUploadMaxTotalMB = "upload.max-total-size"
	KeyUploadAllowMedia = "upload.allow-media"
	KeyUploadBlockedExt = "upload.blocked-extensions"

	KeyCommentsEnabled     = "comments.enabled"
	KeyCommentsNotifyOwner = "comments.notify-owner"
	KeyCommentsAllowGuests = "comments.allow-guests"

	KeySearchReindexDays = "search.reindex-days"
	KeySearchLastIndex   = "search.last-index" // internal, not exposed in the UI
	KeySearchDefault     = "search.default"
	KeyIndexType         = "index.type"
	KeyMeiliHost         = "index.meili.host"
	KeyMeiliAPIKey       = "index.meili.api-key"

	KeyCORSEnabled     = "security.cors-enabled"
	KeyCORSOrigins     = "security.cors-origins"
	KeyCORSCredentials = "security.cors-allow-credentials"

	KeyModerationEnabled  = "moderation.enabled"
	KeyModerationEndpoint = "moderation.endpoint"
	KeyModerationAPIKey   = "moderation.api-key"
	KeyModerationModel    = "moderation.model"
	KeyModerationPrompt   = "moderation.prompt"
	KeyModerationTimeout  = "moderation.timeout"
	KeyModerationFailSafe = "moderation.fail-closed"
	// KeyModerationReasoning is the provider's reasoning effort. The "none"
	// value leaves the field out of the request, which is what models without
	// reasoning support need.
	KeyModerationReasoning = "moderation.reasoning"
	// KeyModerationMaxOutput bounds the answer so one runaway response cannot
	// consume the whole context window or the bill.
	KeyModerationMaxOutput = "moderation.max-output-tokens"
	// KeyModerationMaxInputKB bounds the document sent to the provider.
	KeyModerationMaxInputKB = "moderation.max-input-size"
	// KeyModerationDailyLimit caps provider calls per calendar day. Gists that
	// arrive after the cap stay pending and are checked later; 0 means no cap.
	KeyModerationDailyLimit = "moderation.daily-limit"

	KeyGitDefaultBranch = "git.default-branch"
	KeyGitHTTPEnabled   = "git.http-enabled"
	// Clone URLs shown under the SSH tab. Empty means "use the host the
	// request arrived on", which is right for most installs, but a site behind
	// a proxy or reachable through a dedicated SSH hostname wants its own.
	KeyGitSSHExternalDomain = "git.ssh-external-domain"
	KeyGitSSHUsername       = "git.ssh-username"
	KeyAPIEnabled           = "api.enabled"

	// Registration modes.
	RegistrationModeOpen      = "open"
	RegistrationModeOAuthOnly = "oauth-only"
	RegistrationModeClosed    = "closed"
)

// DefaultRepositoryURL is the project this instance is built from. It is shown
// as a source-code icon in the public sidebar and can be changed per instance.
const DefaultRepositoryURL = "https://github.com/qingyu-org/Gist"

// DefaultGitLabURL is used when neither the setting nor the environment names
// a GitLab instance; without it the provider would build relative endpoints.
const DefaultGitLabURL = "https://gitlab.com"

// DefaultLDAPSearchFilter matches the username against the uid attribute. The
// single %s is replaced with the login name.
const DefaultLDAPSearchFilter = "(uid=%s)"

// DefaultModerationPrompt is used when the administrator has not customised it.
const DefaultModerationPrompt = `You are the content moderation system of a self-hosted code snippet site.
Decide whether the submitted gist may be published publicly.
Reject content that is illegal, sexual, hateful, harassing, promotes violence, contains
malware, phishing pages, credential dumps or spam. Code snippets are allowed even when
they are dual-use; reject only content that is clearly malicious or abusive.
The answer format is enforced by the request, so explain the verdict in one short sentence.`

// SecretPlaceholder is rendered in place of a stored secret. Submitting it
// unchanged keeps the existing value.
const SecretPlaceholder = "********"

// Option is a choice for an enum setting.
type Option struct {
	Value string
	Label string // i18n key
}

// Definition describes one configurable setting.
type Definition struct {
	Key      string
	Kind     Kind
	Default  string
	Category Category
	Label    string // i18n key
	Help     string // i18n key
	// Provider is set on third-party sign-in fields only, to the provider key
	// ("github", "oidc", …). The admin panel groups those fields into tabs.
	Provider    string
	Placeholder string
	Options     []Option
	Min         int
	Max         int
	Unit        string // i18n key, e.g. "settings.unit.mb"
	Rows        int
	// ApplyAfterRestart marks settings that cannot take effect without a
	// restart. They are still stored but shown with a note in the UI.
	ApplyAfterRestart bool
}

var baseDefinitions = []Definition{
	// Site / branding
	{Key: KeySiteName, Kind: KindString, Category: CategorySite, Label: "settings.site.name", Help: "settings.site.name_help", Placeholder: "QingyuGist", Max: 80},
	{Key: KeySiteDescription, Kind: KindString, Category: CategorySite, Label: "settings.site.description", Help: "settings.site.description_help", Max: 200},
	{Key: KeySiteExternalURL, Kind: KindURL, Category: CategorySite, Label: "settings.site.external_url", Help: "settings.site.external_url_help", Placeholder: "https://gist.example.com", Max: 300},
	{Key: KeySiteLogo, Kind: KindString, Category: CategorySite, Label: "settings.site.logo", Help: "settings.site.logo_help", Placeholder: "logo.svg", Max: 300},
	{Key: KeySiteFavicon, Kind: KindString, Category: CategorySite, Label: "settings.site.favicon", Help: "settings.site.favicon_help", Placeholder: "favicon.png", Max: 300},
	{Key: KeySiteHomepage, Kind: KindURL, Category: CategorySite, Label: "settings.site.homepage_url", Help: "settings.site.homepage_url_help", Placeholder: "https://example.com", Max: 300},
	{Key: KeySiteEmail, Kind: KindString, Default: "", Category: CategorySite, Label: "settings.site.contact_email", Help: "settings.site.contact_email_help", Placeholder: "hello@example.com", Max: 200},
	{Key: KeySiteICP, Kind: KindString, Default: "", Category: CategorySite, Label: "settings.site.icp", Help: "settings.site.icp_help", Placeholder: "京ICP备 00000000 号-1", Max: 80},
	{Key: KeySitePolice, Kind: KindString, Default: "", Category: CategorySite, Label: "settings.site.police_record", Help: "settings.site.police_record_help", Placeholder: "京公网安备 00000000000000 号", Max: 80},
	{Key: KeySiteCustomCSS, Kind: KindText, Category: CategorySite, Label: "settings.site.custom_css", Help: "settings.site.custom_css_help", Rows: 6},
	{Key: KeySiteCustomJS, Kind: KindText, Category: CategorySite, Label: "settings.site.custom_js", Help: "settings.site.custom_js_help", Rows: 6},
	{Key: KeySiteRepository, Kind: KindURL, Default: DefaultRepositoryURL, Category: CategorySite, Label: "settings.site.repository_url", Help: "settings.site.repository_url_help", Placeholder: DefaultRepositoryURL, Max: 300},
	{Key: KeySiteLinks, Kind: KindText, Category: CategorySite, Label: "settings.site.links", Help: "settings.site.links_help", Placeholder: "[]", Rows: 4},
	{Key: KeySiteAnnouncement, Kind: KindText, Category: CategorySite, Label: "settings.site.announcement", Help: "settings.site.announcement_help", Rows: 5},

	// Authentication & registration
	{Key: KeyRegistrationMode, Kind: KindEnum, Category: CategoryAuth, Default: RegistrationModeOpen, Label: "settings.auth.registration_mode", Help: "settings.auth.registration_mode_help", Options: []Option{
		{Value: RegistrationModeOpen, Label: "settings.auth.registration_open"},
		{Value: RegistrationModeOAuthOnly, Label: "settings.auth.registration_oauth_only"},
		{Value: RegistrationModeClosed, Label: "settings.auth.registration_closed"},
	}},
	{Key: KeyRequireLogin, Kind: KindBool, Category: CategoryAuth, Label: "settings.auth.require_login", Help: "settings.auth.require_login_help"},
	{Key: KeyAllowGistsWithoutLogin, Kind: KindBool, Category: CategoryAuth, Label: "settings.auth.allow_gists_without_login", Help: "settings.auth.allow_gists_without_login_help"},
	{Key: KeyDisableLoginForm, Kind: KindBool, Category: CategoryAuth, Label: "settings.auth.disable_login_form", Help: "settings.auth.disable_login_form_help"},
	{Key: KeyAvatarProvider, Kind: KindEnum, Default: AvatarProviderWeAvatar, Category: CategoryAuth, Label: "settings.auth.avatar_provider", Help: "settings.auth.avatar_provider_help", Options: []Option{
		{Value: AvatarProviderWeAvatar, Label: "settings.auth.avatar_weavatar"},
		{Value: AvatarProviderGravatar, Label: "settings.auth.avatar_gravatar"},
		{Value: AvatarProviderNone, Label: "settings.auth.avatar_generated"},
	}},
	{Key: KeyAvatarDefault, Kind: KindURL, Category: CategoryAuth, Label: "settings.auth.avatar_default", Help: "settings.auth.avatar_default_help", Placeholder: "https://example.com/avatar.png", Max: 300},

	// Third-party sign-in lives in oauth.go: one enable switch plus credentials
	// per provider, appended to the definitions below.

	// LDAP. Sign-in through LDAP is enabled as soon as the URL is set.
	{Key: KeyLDAPURL, Kind: KindString, Category: CategoryLDAP, Label: "settings.ldap.url", Help: "settings.ldap.url_help", Placeholder: "ldap://localhost:389", Max: 300},
	{Key: KeyLDAPBindDN, Kind: KindString, Category: CategoryLDAP, Label: "settings.ldap.bind_dn", Help: "settings.ldap.bind_dn_help", Placeholder: "cn=admin,dc=example,dc=com", Max: 300},
	{Key: KeyLDAPBindPassword, Kind: KindSecret, Category: CategoryLDAP, Label: "settings.ldap.bind_password", Help: "settings.ldap.bind_password_help", Max: 300},
	{Key: KeyLDAPSearchBase, Kind: KindString, Category: CategoryLDAP, Label: "settings.ldap.search_base", Help: "settings.ldap.search_base_help", Placeholder: "ou=users,dc=example,dc=com", Max: 300},
	{Key: KeyLDAPSearchFilter, Kind: KindString, Default: DefaultLDAPSearchFilter, Category: CategoryLDAP, Label: "settings.ldap.search_filter", Help: "settings.ldap.search_filter_help", Placeholder: DefaultLDAPSearchFilter, Max: 300},

	// Uploads & storage
	{Key: KeyUploadEnabled, Kind: KindBool, Default: "1", Category: CategoryUpload, Label: "settings.upload.enabled", Help: "settings.upload.enabled_help"},
	{Key: KeyUploadMaxFileMB, Kind: KindInt, Default: "32", Category: CategoryUpload, Label: "settings.upload.max_file_size", Help: "settings.upload.max_file_size_help", Min: 1, Max: 4096, Unit: "settings.unit.mb"},
	{Key: KeyUploadMaxTotalMB, Kind: KindInt, Default: "128", Category: CategoryUpload, Label: "settings.upload.max_total_size", Help: "settings.upload.max_total_size_help", Min: 1, Max: 65536, Unit: "settings.unit.mb"},
	{Key: KeyUploadAllowMedia, Kind: KindBool, Default: "1", Category: CategoryUpload, Label: "settings.upload.allow_media", Help: "settings.upload.allow_media_help"},
	{Key: KeyUploadBlockedExt, Kind: KindText, Default: "exe,msi,dll,so,dylib,iso,dmg,img,bin,apk,ipa,zip,7z,rar,tar,gz,bz2,xz,zst", Category: CategoryUpload, Label: "settings.upload.blocked_extensions", Help: "settings.upload.blocked_extensions_help", Rows: 2},

	// Comments
	{Key: KeyCommentsEnabled, Kind: KindBool, Default: "1", Category: CategoryComments, Label: "settings.comments.enabled", Help: "settings.comments.enabled_help"},
	{Key: KeyCommentsNotifyOwner, Kind: KindBool, Default: "1", Category: CategoryComments, Label: "settings.comments.notify_owner", Help: "settings.comments.notify_owner_help"},
	{Key: KeyCommentsAllowGuests, Kind: KindBool, Default: "0", Category: CategoryComments, Label: "settings.comments.allow_guests", Help: "settings.comments.allow_guests_help"},

	// Search & index
	{Key: KeyIndexType, Kind: KindEnum, Default: "bleve", ApplyAfterRestart: true, Category: CategorySearch, Label: "settings.search.index_type", Help: "settings.search.index_type_help", Options: []Option{
		{Value: "bleve", Label: "settings.search.index_bleve"},
		{Value: "meilisearch", Label: "settings.search.index_meilisearch"},
		{Value: "none", Label: "settings.search.index_none"},
	}},
	{Key: KeyMeiliHost, Kind: KindURL, ApplyAfterRestart: true, Category: CategorySearch, Label: "settings.search.meili_host", Help: "settings.search.meili_host_help", Placeholder: "http://localhost:7700", Max: 300},
	{Key: KeyMeiliAPIKey, Kind: KindSecret, ApplyAfterRestart: true, Category: CategorySearch, Label: "settings.search.meili_api_key", Help: "settings.search.meili_api_key_help", Max: 300},
	{Key: KeySearchDefault, Kind: KindString, Default: "content", Category: CategorySearch, Label: "settings.search.default_fields", Help: "settings.search.default_fields_help", Placeholder: "content", Max: 200},
	{Key: KeySearchReindexDays, Kind: KindInt, Default: "7", Category: CategorySearch, Label: "settings.search.reindex_days", Help: "settings.search.reindex_days_help", Min: 0, Max: 365, Unit: "settings.unit.days"},

	// Security
	{Key: KeyCORSEnabled, Kind: KindBool, Default: "0", Category: CategorySecurity, Label: "settings.security.cors_enabled", Help: "settings.security.cors_enabled_help"},
	{Key: KeyCORSOrigins, Kind: KindText, Default: "*", Category: CategorySecurity, Label: "settings.security.cors_origins", Help: "settings.security.cors_origins_help", Placeholder: "https://example.com", Rows: 3},
	{Key: KeyCORSCredentials, Kind: KindBool, Default: "0", Category: CategorySecurity, Label: "settings.security.cors_credentials", Help: "settings.security.cors_credentials_help"},

	// AI moderation
	{Key: KeyModerationEnabled, Kind: KindBool, Default: "0", Category: CategoryModeration, Label: "settings.moderation.enabled", Help: "settings.moderation.enabled_help"},
	{Key: KeyModerationEndpoint, Kind: KindURL, Category: CategoryModeration, Label: "settings.moderation.endpoint", Help: "settings.moderation.endpoint_help", Placeholder: "https://api.openai.com/v1/responses", Max: 500},
	{Key: KeyModerationAPIKey, Kind: KindSecret, Category: CategoryModeration, Label: "settings.moderation.api_key", Help: "settings.moderation.api_key_help", Max: 500},
	{Key: KeyModerationModel, Kind: KindString, Default: "gpt-4o-mini", Category: CategoryModeration, Label: "settings.moderation.model", Help: "settings.moderation.model_help", Max: 120},
	{Key: KeyModerationReasoning, Kind: KindEnum, Default: "none", Category: CategoryModeration, Label: "settings.moderation.reasoning", Help: "settings.moderation.reasoning_help", Options: []Option{
		{Value: "none", Label: "settings.moderation.reasoning_default"},
		{Value: "low", Label: "settings.moderation.reasoning_low"},
		{Value: "medium", Label: "settings.moderation.reasoning_medium"},
		{Value: "high", Label: "settings.moderation.reasoning_high"},
	}},
	{Key: KeyModerationMaxOutput, Kind: KindInt, Default: "400", Category: CategoryModeration, Label: "settings.moderation.max_output", Help: "settings.moderation.max_output_help", Min: 64, Max: 8192, Unit: "settings.unit.tokens"},
	{Key: KeyModerationMaxInputKB, Kind: KindInt, Default: "16", Category: CategoryModeration, Label: "settings.moderation.max_input", Help: "settings.moderation.max_input_help", Min: 1, Max: 256, Unit: "settings.unit.kb"},
	{Key: KeyModerationDailyLimit, Kind: KindInt, Default: "200", Category: CategoryModeration, Label: "settings.moderation.daily_limit", Help: "settings.moderation.daily_limit_help", Min: 0, Max: 100000, Unit: "settings.unit.per_day"},
	{Key: KeyModerationTimeout, Kind: KindInt, Default: "45", Category: CategoryModeration, Label: "settings.moderation.timeout", Help: "settings.moderation.timeout_help", Min: 5, Max: 300, Unit: "settings.unit.seconds"},
	{Key: KeyModerationFailSafe, Kind: KindBool, Default: "1", Category: CategoryModeration, Label: "settings.moderation.fail_closed", Help: "settings.moderation.fail_closed_help"},
	{Key: KeyModerationPrompt, Kind: KindText, Default: DefaultModerationPrompt, Category: CategoryModeration, Label: "settings.moderation.prompt", Help: "settings.moderation.prompt_help", Rows: 8},

	// Git / API
	{Key: KeyGitDefaultBranch, Kind: KindString, Category: CategoryGit, Label: "settings.git.default_branch", Help: "settings.git.default_branch_help", Placeholder: "main", Max: 100},
	{Key: KeyGitHTTPEnabled, Kind: KindBool, Default: "1", ApplyAfterRestart: true, Category: CategoryGit, Label: "settings.git.http_enabled", Help: "settings.git.http_enabled_help"},
	{Key: KeyGitSSHExternalDomain, Kind: KindString, Category: CategoryGit, Label: "settings.git.ssh_external_domain", Help: "settings.git.ssh_external_domain_help", Placeholder: "git.example.com", Max: 255},
	{Key: KeyGitSSHUsername, Kind: KindString, Category: CategoryGit, Label: "settings.git.ssh_username", Help: "settings.git.ssh_username_help", Placeholder: "git", Max: 64},
	{Key: KeyAPIEnabled, Kind: KindBool, Default: "1", Category: CategoryAPI, Label: "settings.api.enabled", Help: "settings.api.enabled_help"},
}

var definitions = append(baseDefinitions, oauthDefinitions()...)

var (
	byKey = make(map[string]*Definition, len(definitions))
	// values is an immutable snapshot; writes replace the whole map.
	values atomic.Pointer[map[string]string]
	// loadedVersion is the db.SettingsVersion the snapshot was built from.
	loadedVersion atomic.Uint64
	// mu serialises reload/apply so two concurrent saves cannot interleave.
	mu sync.Mutex
)

func init() {
	for i := range definitions {
		byKey[definitions[i].Key] = &definitions[i]
	}
	empty := map[string]string{}
	values.Store(&empty)

	// Lets the API shapes report the same picture the templates draw (see
	// db.AvatarURLBuilder — db cannot import this package).
	db.AvatarURLBuilder = AvatarURL
}

// Definitions returns all setting definitions in declaration order.
func Definitions() []Definition {
	return definitions
}

// AllDefinitions returns every configurable setting in display order.
func AllDefinitions() []Definition {
	return append([]Definition(nil), definitions...)
}

// DefinitionsByCategory returns the definitions of a category.
func DefinitionsByCategory(c Category) []Definition {
	out := make([]Definition, 0, len(definitions))
	for _, d := range definitions {
		if d.Category == c {
			out = append(out, d)
		}
	}
	return out
}

// Define returns a definition by key.
func Define(key string) (Definition, bool) {
	d, ok := byKey[key]
	if !ok {
		return Definition{}, false
	}
	return *d, true
}

// ResolveKey maps a legacy admin-setting key written by older releases to its
// canonical dotted key. Unknown keys are returned unchanged.
func ResolveKey(key string) string {
	switch key {
	case db.SettingRequireLogin:
		return KeyRequireLogin
	case db.SettingAllowGistsWithoutLogin:
		return KeyAllowGistsWithoutLogin
	case db.SettingDisableLoginForm:
		return KeyDisableLoginForm
	}
	return key
}

// NormalizeLegacyValue converts a value sent for a legacy key into the
// representation used by the canonical setting. Only registration mode has a
// different shape: the old API used a boolean "disable-signup" flag.
func NormalizeLegacyValue(key, value string) (string, string) {
	if key == db.SettingDisableSignup && value == "1" {
		return KeyRegistrationMode, RegistrationModeClosed
	}
	if key == db.SettingDisableSignup {
		return KeyRegistrationMode, RegistrationModeOpen
	}
	// The old boolean switched Gravatar off entirely; its successor is the
	// "generated avatars only" provider.
	if key == db.SettingDisableGravatar {
		if value == "1" {
			return KeyAvatarProvider, AvatarProviderNone
		}
		return KeyAvatarProvider, AvatarProviderWeAvatar
	}
	return ResolveKey(key), value
}

// Init seeds missing defaults (including values imported from the legacy
// config file/admin settings) and loads the in-memory snapshot. Call it once
// after the database is ready and before the HTTP server starts.
func Init() error {
	mu.Lock()
	defer mu.Unlock()

	existing, err := db.GetSettings()
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}

	defaults := defaultValues()
	for _, d := range definitions {
		if _, ok := existing[d.Key]; ok {
			continue
		}
		value, ok := defaults[d.Key]
		if !ok {
			value = d.Default
		}
		if err := db.UpdateSetting(d.Key, value); err != nil {
			return fmt.Errorf("seed setting %s: %w", d.Key, err)
		}
		existing[d.Key] = value
	}

	// Keep the legacy keys consistent for code paths that still read them
	// (and for a clean downgrade).
	if err := mirrorLegacy(existing); err != nil {
		return err
	}

	values.Store(&existing)
	loadedVersion.Store(db.SettingsVersion())
	applyLocked(existing)
	return nil
}

// Reload re-reads all values from the database and applies them.
func Reload() error {
	mu.Lock()
	defer mu.Unlock()

	existing, err := db.GetSettings()
	if err != nil {
		return err
	}
	values.Store(&existing)
	loadedVersion.Store(db.SettingsVersion())
	applyLocked(existing)
	return nil
}

// Reset drops the cache and re-seeds it. It exists for tests and for tooling
// that swaps the database underneath the process.
func Reset() error {
	values.Store(&map[string]string{})
	return Init()
}

// EnsureFresh reloads the snapshot when another code path wrote a setting
// directly through the db package.
func EnsureFresh() error {
	if loadedVersion.Load() == db.SettingsVersion() {
		return nil
	}
	if err := Reload(); err != nil {
		log.Warn().Err(err).Msg("Could not refresh settings cache")
		return err
	}
	return nil
}

// defaultValues derives defaults that depend on the boot configuration or on
// settings written by older releases.
func defaultValues() map[string]string {
	legacy, _ := db.GetSettings()
	get := func(key string) string { return legacy[key] }

	registration := RegistrationModeOpen
	if get(db.SettingDisableSignup) == "1" {
		registration = RegistrationModeClosed
	}

	uploadEnabled := "1"
	if config.C.DisableFileUpload {
		uploadEnabled = "0"
	}

	apiEnabled := "0"
	if config.C.ApiEnabled {
		apiEnabled = "1"
	}

	commentsEnabled := "0"
	if config.C.CommentsEnabled {
		commentsEnabled = "1"
	}

	commentNotify := "0"
	if config.C.CommentNotify {
		commentNotify = "1"
	}

	commentsAllowGuests := "0"
	if config.C.CommentsAllowGuests {
		commentsAllowGuests = "1"
	}

	boolFromLegacy := func(key, fallback string) string {
		if v, ok := legacy[key]; ok {
			return v
		}
		return fallback
	}

	nonEmpty := func(value, fallback string) string {
		if value != "" {
			return value
		}
		return fallback
	}

	httpGit := "0"
	if config.C.HttpGit {
		httpGit = "1"
	}

	// An install that had switched Gravatar off keeps generated avatars after
	// the upgrade; the old boolean has no other successor.
	avatarProvider := AvatarProviderWeAvatar
	if boolFromLegacy(db.SettingDisableGravatar, "0") == "1" {
		avatarProvider = AvatarProviderNone
	}

	values := map[string]string{
		KeySiteName:               config.C.CustomName,
		KeySiteExternalURL:        config.C.ExternalUrl,
		KeySiteLogo:               config.C.CustomLogo,
		KeySiteFavicon:            config.C.CustomFavicon,
		KeyRegistrationMode:       registration,
		KeyRequireLogin:           boolFromLegacy(db.SettingRequireLogin, "0"),
		KeyAllowGistsWithoutLogin: boolFromLegacy(db.SettingAllowGistsWithoutLogin, "0"),
		KeyDisableLoginForm:       boolFromLegacy(db.SettingDisableLoginForm, "0"),
		KeyAvatarProvider:         avatarProvider,
		KeyUploadEnabled:          uploadEnabled,
		KeyCommentsEnabled:        commentsEnabled,
		KeyCommentsNotifyOwner:    commentNotify,
		KeyCommentsAllowGuests:    commentsAllowGuests,
		KeyGitDefaultBranch:       config.C.GitDefaultBranch,
		KeyGitHTTPEnabled:         httpGit,
		KeyGitSSHExternalDomain:   config.C.SshExternalDomain,
		KeyGitSSHUsername:         config.C.SshUsername,
		KeyAPIEnabled:             apiEnabled,

		KeyLDAPURL:          config.C.LDAPUrl,
		KeyLDAPBindDN:       config.C.LDAPBindDn,
		KeyLDAPBindPassword: config.C.LDAPBindCredentials,
		KeyLDAPSearchBase:   config.C.LDAPSearchBase,
		KeyLDAPSearchFilter: nonEmpty(config.C.LDAPSearchFilter, DefaultLDAPSearchFilter),

		KeyIndexType:     nonEmpty(config.C.Index, "none"),
		KeyMeiliHost:     config.C.MeiliHost,
		KeyMeiliAPIKey:   config.C.MeiliAPIKey,
		KeySearchDefault: config.C.SearchDefault,
	}

	// Third-party sign-in: seed every provider from whatever the environment
	// provided, so an env-only deployment keeps working after the upgrade.
	for _, d := range oauthDefinitions() {
		field := strings.TrimPrefix(d.Key, "oauth."+d.Provider+".")
		if value, ok := oauthConfigValue(config.C, d.Provider, field); ok {
			values[d.Key] = value
		}
	}

	return values
}

func mirrorLegacy(v map[string]string) error {
	pairs := map[string]string{
		KeyRequireLogin:           db.SettingRequireLogin,
		KeyAllowGistsWithoutLogin: db.SettingAllowGistsWithoutLogin,
		KeyDisableLoginForm:       db.SettingDisableLoginForm,
	}
	for key, legacy := range pairs {
		val := v[key]
		if val == "" {
			val = "0"
		}
		if err := db.UpdateSetting(legacy, val); err != nil {
			return err
		}
	}

	disableSignup := "0"
	if v[KeyRegistrationMode] == RegistrationModeClosed {
		disableSignup = "1"
	}
	if err := db.UpdateSetting(db.SettingDisableSignup, disableSignup); err != nil {
		return err
	}

	disableGravatar := "0"
	if v[KeyAvatarProvider] == AvatarProviderNone {
		disableGravatar = "1"
	}
	return db.UpdateSetting(db.SettingDisableGravatar, disableGravatar)
}

// applyLocked copies the boot config and overlays the settings that mirror it.
// Replacing config.C (rather than mutating it) keeps concurrent readers safe:
// they observe either the old or the new snapshot, never a half-written value.
func applyLocked(v map[string]string) {
	next := *config.C
	trimmed := func(key string) string { return strings.TrimSpace(v[key]) }

	next.CustomName = v[KeySiteName]
	next.CustomLogo = v[KeySiteLogo]
	next.CustomFavicon = v[KeySiteFavicon]
	if u := strings.TrimRight(strings.TrimSpace(v[KeySiteExternalURL]), "/"); u != "" {
		next.ExternalUrl = u
	}
	if d := strings.TrimSpace(v[KeySearchDefault]); d != "" {
		next.SearchDefault = d
	}
	if b := strings.TrimSpace(v[KeyGitDefaultBranch]); b != "" {
		next.GitDefaultBranch = b
	}
	next.ApiEnabled = v[KeyAPIEnabled] == "1"
	next.CommentsEnabled = v[KeyCommentsEnabled] == "1"
	next.CommentNotify = v[KeyCommentsNotifyOwner] == "1"
	next.CommentsAllowGuests = v[KeyCommentsAllowGuests] == "1"
	next.DisableFileUpload = v[KeyUploadEnabled] != "1"
	next.HttpGit = v[KeyGitHTTPEnabled] != "0"
	next.SshExternalDomain = trimmed(KeyGitSSHExternalDomain)
	next.SshUsername = trimmed(KeyGitSSHUsername)

	applyOAuthSettings(&next, v)

	// LDAP. Unlike the credentials above, clearing the URL has to take effect:
	// an empty URL is how sign-in through LDAP is switched off.
	next.LDAPUrl = trimmed(KeyLDAPURL)
	next.LDAPBindDn = trimmed(KeyLDAPBindDN)
	if s := v[KeyLDAPBindPassword]; s != "" {
		next.LDAPBindCredentials = s
	}
	next.LDAPSearchBase = trimmed(KeyLDAPSearchBase)
	if s := trimmed(KeyLDAPSearchFilter); s != "" {
		next.LDAPSearchFilter = s
	} else {
		next.LDAPSearchFilter = DefaultLDAPSearchFilter
	}

	// Search index
	if s := trimmed(KeyMeiliHost); s != "" {
		next.MeiliHost = s
	}
	if s := v[KeyMeiliAPIKey]; s != "" {
		next.MeiliAPIKey = s
	}
	switch trimmed(KeyIndexType) {
	case "bleve":
		next.Index = "bleve"
	case "meilisearch":
		next.Index = "meilisearch"
	case "none":
		next.Index = ""
	}

	config.C = &next
}

// Validate checks a candidate value against its definition.
func Validate(key, value string) error {
	d, ok := byKey[key]
	if !ok {
		return fmt.Errorf("unknown setting %q", key)
	}
	return validateDefinition(*d, value)
}

func validateDefinition(d Definition, value string) error {
	if d.Max > 0 && len(value) > d.Max {
		return fmt.Errorf("%s: value is too long (max %d)", d.Key, d.Max)
	}
	if d.Key == KeySiteLinks {
		if _, err := parseSiteLinks(value); err != nil {
			return err
		}
	}
	if d.Key == KeySiteEmail {
		if value = strings.TrimSpace(value); value != "" {
			addr, err := mail.ParseAddress(value)
			if err != nil || addr.Address != value {
				return fmt.Errorf("%s: %q is not a valid email address", d.Key, value)
			}
		}
	}
	switch d.Kind {
	case KindBool:
		if value != "0" && value != "1" {
			return fmt.Errorf("%s: expected 0 or 1", d.Key)
		}
	case KindInt:
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s: expected a number", d.Key)
		}
		if n < d.Min || n > d.Max {
			return fmt.Errorf("%s: must be between %d and %d", d.Key, d.Min, d.Max)
		}
	case KindEnum:
		found := false
		for _, o := range d.Options {
			if o.Value == value {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: invalid value", d.Key)
		}
	case KindURL:
		if strings.TrimSpace(value) == "" {
			return nil
		}
		u, err := url.Parse(strings.TrimSpace(value))
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("%s: enter a valid absolute URL", d.Key)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("%s: only http(s) URLs are allowed", d.Key)
		}
	}
	return nil
}

// Set validates, persists and applies one setting.
func Set(key, value string) error {
	return SetMany(map[string]string{key: value})
}

// SetMany validates every pair before writing any of them, then persists the
// batch, refreshes the cache and re-applies runtime overrides.
func SetMany(pairs map[string]string) error {
	for key, value := range pairs {
		if err := Validate(key, value); err != nil {
			return err
		}
	}

	mu.Lock()
	defer mu.Unlock()

	updated, err := db.GetSettings()
	if err != nil {
		return err
	}
	for key, value := range pairs {
		updated[key] = value
	}

	if err := db.UpdateSettings(pairs); err != nil {
		return err
	}
	if err := mirrorLegacy(updated); err != nil {
		return err
	}

	values.Store(&updated)
	loadedVersion.Store(db.SettingsVersion())
	applyLocked(updated)
	return nil
}

// SetInternal writes a bookkeeping value (last index run, ...) that is not a
// user-facing setting. It bypasses definitions on purpose.
func SetInternal(key, value string) error {
	if err := db.UpdateSetting(key, value); err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	current, err := db.GetSettings()
	if err != nil {
		return err
	}
	values.Store(&current)
	loadedVersion.Store(db.SettingsVersion())
	return nil
}

// Get returns the current raw value, or the definition default.
func Get(key string) string {
	if v := values.Load(); v != nil {
		if val, ok := (*v)[key]; ok {
			return val
		}
	}
	if d, ok := byKey[key]; ok {
		return d.Default
	}
	return ""
}

// Bool returns the setting as a boolean ("1" is true).
func Bool(key string) bool { return Get(key) == "1" }

// Int returns the setting as an integer, falling back to the definition default.
func Int(key string) int {
	v := Get(key)
	n, err := strconv.Atoi(v)
	if err != nil {
		if d, ok := byKey[key]; ok {
			n, _ = strconv.Atoi(d.Default)
		}
	}
	return n
}

// Snapshot returns a copy of all current values.
func Snapshot() map[string]string {
	src := values.Load()
	if src == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(*src))
	for k, v := range *src {
		out[k] = v
	}
	return out
}

// ---- Typed helpers used across the app ----

func SiteName() string {
	if n := strings.TrimSpace(Get(KeySiteName)); n != "" {
		return n
	}
	if config.C.CustomName != "" {
		return config.C.CustomName
	}
	return "QingyuGist"
}

func SiteDescription() string { return strings.TrimSpace(Get(KeySiteDescription)) }

func SiteRepositoryURL() string { return strings.TrimSpace(Get(KeySiteRepository)) }

func SiteHomepageURL() string { return strings.TrimSpace(Get(KeySiteHomepage)) }

func SiteContactEmail() string { return strings.TrimSpace(Get(KeySiteEmail)) }

func SiteICP() string { return strings.TrimSpace(Get(KeySiteICP)) }

func SitePoliceRecord() string { return strings.TrimSpace(Get(KeySitePolice)) }

// SiteAnnouncement is the markdown banner shown across the site. Empty means
// no banner.
func SiteAnnouncement() string { return strings.TrimSpace(Get(KeySiteAnnouncement)) }

// SiteLink is an administrator-defined link shown at the bottom of the sidebar.
type SiteLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Icon  string `json:"icon"`
}

var siteLinkIcons = []string{
	"link", "home", "globe", "github", "gitlab", "gitea", "mail",
	"rss", "code", "book", "user", "x", "mastodon", "telegram",
}

// SiteLinkIcons lists the icon names accepted by the link editor and rendered
// by templates/partials/site_icons.html.
func SiteLinkIcons() []string { return append([]string(nil), siteLinkIcons...) }

func validSiteLinkIcon(icon string) bool {
	for _, name := range siteLinkIcons {
		if name == icon {
			return true
		}
	}
	return false
}

func parseSiteLinks(raw string) ([]SiteLink, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" || raw == "null" {
		return nil, nil
	}
	var items []SiteLink
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("%s: expected a JSON list of links", KeySiteLinks)
	}
	if len(items) > 12 {
		return nil, fmt.Errorf("%s: at most 12 links are allowed", KeySiteLinks)
	}
	out := make([]SiteLink, 0, len(items))
	for _, item := range items {
		item.Label = strings.TrimSpace(item.Label)
		item.URL = strings.TrimSpace(item.URL)
		item.Icon = strings.ToLower(strings.TrimSpace(item.Icon))
		if item.Label == "" || item.URL == "" {
			return nil, fmt.Errorf("%s: every link needs a label and a URL", KeySiteLinks)
		}
		if utf8.RuneCountInString(item.Label) > 40 {
			return nil, fmt.Errorf("%s: link labels are limited to 40 characters", KeySiteLinks)
		}
		if len(item.URL) > 500 || !validSiteLinkURL(item.URL) {
			return nil, fmt.Errorf("%s: %q is not a valid link", KeySiteLinks, item.URL)
		}
		if item.Icon == "" {
			item.Icon = "link"
		}
		if !validSiteLinkIcon(item.Icon) {
			item.Icon = "link"
		}
		out = append(out, item)
	}
	return out, nil
}

func validSiteLinkURL(value string) bool {
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" {
		return false
	}
	switch u.Scheme {
	case "http", "https":
		return u.Host != ""
	case "mailto":
		return u.Opaque != "" || u.Path != ""
	}
	return false
}

// ParseSiteLinks decodes and validates the JSON value used by the sidebar link
// editor. It is exported so the admin form can render the submitted value again
// after a validation error.
func ParseSiteLinks(raw string) ([]SiteLink, error) {
	return parseSiteLinks(raw)
}

// SiteLinks returns the configured footer links, skipping a malformed value
// rather than breaking every page for a bad manual edit.
func SiteLinks() []SiteLink {
	links, err := parseSiteLinks(Get(KeySiteLinks))
	if err != nil {
		log.Warn().Err(err).Msg("Ignoring malformed site link configuration")
		return nil
	}
	return links
}

func CustomCSS() string { return Get(KeySiteCustomCSS) }
func CustomJS() string  { return Get(KeySiteCustomJS) }

func RegistrationMode() string {
	mode := Get(KeyRegistrationMode)
	switch mode {
	case RegistrationModeOpen, RegistrationModeOAuthOnly, RegistrationModeClosed:
		return mode
	default:
		return RegistrationModeOpen
	}
}

func RegistrationOpenEnabled() bool { return RegistrationMode() == RegistrationModeOpen }
func OAuthOnlyRegistration() bool   { return RegistrationMode() == RegistrationModeOAuthOnly }
func ClosedRegistration() bool      { return RegistrationMode() == RegistrationModeClosed }
func SignupEnabled() bool           { return !ClosedRegistration() }

func RequireLogin() bool           { return Bool(KeyRequireLogin) }
func AllowGistsWithoutLogin() bool { return Bool(KeyAllowGistsWithoutLogin) }
func DisableLoginForm() bool       { return Bool(KeyDisableLoginForm) }

// Avatar providers. Both are Gravatar protocol services: the picture is derived
// from the MD5 hash of the account's email address, so the app never stores or
// proxies an image and no provider account has to be linked.
const (
	AvatarProviderWeAvatar = "weavatar"
	AvatarProviderGravatar = "gravatar"
	AvatarProviderNone     = "none"
)

var avatarProviderHosts = map[string]string{
	AvatarProviderWeAvatar: "https://weavatar.com/avatar",
	AvatarProviderGravatar: "https://www.gravatar.com/avatar",
}

var avatarProviderSites = map[string]string{
	AvatarProviderWeAvatar: "https://weavatar.com",
	AvatarProviderGravatar: "https://gravatar.com",
}

// AvatarProvider returns the configured provider key, normalised so a value
// from an older release or a hand-edited row never leaves the app guessing.
func AvatarProvider() string {
	switch Get(KeyAvatarProvider) {
	case AvatarProviderWeAvatar, AvatarProviderGravatar, AvatarProviderNone:
		return Get(KeyAvatarProvider)
	}
	return AvatarProviderWeAvatar
}

// AvatarProviderHost returns the base URL of the configured provider, and ""
// when avatars are generated locally.
func AvatarProviderHost() string { return avatarProviderHosts[AvatarProvider()] }

// AvatarProviderSite returns the provider's home page, where an account holder
// changes their picture. Empty when avatars are generated locally.
func AvatarProviderSite() string { return avatarProviderSites[AvatarProvider()] }

// AvatarProviderDisplayName is the provider's name as shown to end users.
func AvatarProviderDisplayName() string {
	switch AvatarProvider() {
	case AvatarProviderWeAvatar:
		return "WeAvatar"
	case AvatarProviderGravatar:
		return "Gravatar"
	}
	return ""
}

// AvatarDefaultURL is the picture shown for accounts without an email address.
// Empty means they keep the generated identicon.
func AvatarDefaultURL() string { return strings.TrimSpace(Get(KeyAvatarDefault)) }

// AvatarURL builds the address of an account's picture from the Gravatar hash,
// or returns the configured fallback image, or "" when one should be drawn
// locally.
func AvatarURL(hash string) string {
	return avatarURL(AvatarProvider(), AvatarDefaultURL(), hash)
}

// avatarURL is AvatarURL with the settings passed in, so the URL rules can be
// tested without a database.
func avatarURL(provider, fallback, hash string) string {
	fallback = strings.TrimSpace(fallback)
	host := avatarProviderHosts[provider]
	if host == "" || hash == "" {
		return fallback
	}

	// Gravatar and WeAvatar both understand d= for the fallback picture; a URL
	// there must be escaped, a keyword must not.
	src := host + "/" + hash + "?s=200"
	switch {
	case fallback == "":
		return src + "&d=identicon"
	case strings.HasPrefix(fallback, "http://"), strings.HasPrefix(fallback, "https://"):
		return src + "&d=" + url.QueryEscape(fallback)
	default:
		return src + "&d=" + fallback
	}
}

func UploadEnabled() bool      { return Bool(KeyUploadEnabled) }
func UploadMediaAllowed() bool { return Bool(KeyUploadAllowMedia) }
func UploadMaxFileBytes() int64 {
	return int64(Int(KeyUploadMaxFileMB)) << 20
}
func UploadMaxTotalBytes() int64 {
	return int64(Int(KeyUploadMaxTotalMB)) << 20
}

// BlockedExtensions returns the normalised, lower-case extension list.
func BlockedExtensions() []string {
	raw := Get(KeyUploadBlockedExt)
	return splitList(raw)
}

func splitList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == ';'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ToLower(strings.Trim(strings.TrimSpace(f), "."))
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

// ReindexInterval returns 0 when scheduled re-indexing is disabled.
func ReindexInterval() time.Duration {
	days := Int(KeySearchReindexDays)
	if days <= 0 {
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

func LastIndexRun() time.Time {
	raw := Get(KeySearchLastIndex)
	sec, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

func SetLastIndexRun(t time.Time) {
	if err := SetInternal(KeySearchLastIndex, strconv.FormatInt(t.Unix(), 10)); err != nil {
		log.Warn().Err(err).Msg("Could not record index run time")
	}
}

func CORSEnabled() bool     { return Bool(KeyCORSEnabled) }
func CORSCredentials() bool { return Bool(KeyCORSCredentials) }

// CORSOrigins parses the configured origin list. An empty list means "no
// cross-origin access"; a single "*" allows every origin.
func CORSOrigins() []string {
	return splitList(Get(KeyCORSOrigins))
}

func ModerationEnabled() bool {
	return Bool(KeyModerationEnabled) && strings.TrimSpace(Get(KeyModerationEndpoint)) != ""
}

func ModerationEndpoint() string { return strings.TrimSpace(Get(KeyModerationEndpoint)) }
func ModerationAPIKey() string   { return strings.TrimSpace(Get(KeyModerationAPIKey)) }
func ModerationModel() string    { return strings.TrimSpace(Get(KeyModerationModel)) }
func ModerationPrompt() string {
	if p := strings.TrimSpace(Get(KeyModerationPrompt)); p != "" {
		return p
	}
	return DefaultModerationPrompt
}
func ModerationTimeout() time.Duration {
	return time.Duration(Int(KeyModerationTimeout)) * time.Second
}
func ModerationFailClosed() bool { return Bool(KeyModerationFailSafe) }

// ModerationReasoning is the reasoning effort sent to the provider, or "" when
// the field should be left out entirely for models without reasoning support.
func ModerationReasoning() string {
	if v := strings.TrimSpace(Get(KeyModerationReasoning)); v != "" && v != "none" {
		return v
	}
	return ""
}

func ModerationMaxOutputTokens() int { return Int(KeyModerationMaxOutput) }

// ModerationMaxInputBytes is the document budget converted from the KB setting.
func ModerationMaxInputBytes() int { return Int(KeyModerationMaxInputKB) * 1024 }

// ModerationDailyLimit is the provider-call cap per calendar day; 0 disables it.
func ModerationDailyLimit() int { return Int(KeyModerationDailyLimit) }

func APIEnabled() bool { return Bool(KeyAPIEnabled) }
