package config

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/qingyu-org/Gist/internal/session"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var QingyuGistVersion = ""

var C *config

// Config is the runtime configuration snapshot. C always points at the current
// one; the type is aliased so other packages can hold or copy it (the settings
// package overlays stored values onto a copy before publishing it).
type Config = config

var SecretKey []byte

// SSH server modes: the canonical values of ssh.git-enabled. The legacy
// booleans are still accepted (true → builtin, false → disabled).
const (
	SshServerBuiltin  = "builtin"  // QingyuGist's embedded SSH server
	SshServerHost     = "host"     // delegate to the host's OpenSSH
	SshServerDisabled = "disabled" // no SSH git access
)

// The struct stays flat: every option is read from a single flat QYG_* variable.
type config struct {
	SecretKey string `env:"QYG_SECRET_KEY"`

	LogLevel       string `env:"QYG_LOG_LEVEL"`
	LogOutput      string `env:"QYG_LOG_OUTPUT"`
	LogPath        string `env:"QYG_LOG_PATH"`
	ExternalUrl    string `env:"QYG_EXTERNAL_URL"`
	QingyuGistHome string `env:"QYG_HOME"`

	DBUri      string `env:"QYG_DB_URI"`
	DBFilename string `env:"QYG_DB_FILENAME"` // deprecated

	IndexEnabled  bool   `env:"QYG_INDEX_ENABLED"` // deprecated
	Index         string `env:"QYG_INDEX"`
	BleveDirname  string `env:"QYG_INDEX_DIRNAME"` // deprecated
	MeiliHost     string `env:"QYG_MEILI_HOST"`
	MeiliAPIKey   string `env:"QYG_MEILI_API_KEY"`
	SearchDefault string `env:"QYG_SEARCH_DEFAULT"`

	GitDefaultBranch string `env:"QYG_GIT_DEFAULT_BRANCH"`

	SqliteJournalMode string `env:"QYG_SQLITE_JOURNAL_MODE"`

	HttpHost string `env:"QYG_HTTP_HOST"`
	HttpPort string `env:"QYG_HTTP_PORT"`
	HttpGit  bool   `env:"QYG_HTTP_GIT_ENABLED"`

	ApiEnabled bool `env:"QYG_API_ENABLED"`

	DisableFileUpload bool `env:"QYG_DISABLE_FILE_UPLOAD"`

	CommentsEnabled bool `env:"QYG_COMMENTS_ENABLED"`

	// CommentNotify is panel-only: it decides whether the owner of a gist gets
	// an in-site notification when someone comments on it.
	CommentNotify bool

	// CommentsAllowGuests is panel-only: it lets visitors who are not signed in
	// comment, with a name they type in.
	CommentsAllowGuests bool

	UnixSocketPermissions string `env:"QYG_UNIX_SOCKET_PERMISSIONS"`

	SshGit                string `env:"QYG_SSH_GIT_ENABLED"` // builtin | host | disabled (true → builtin, false → disabled)
	SshAuthorizedKeysFile string `env:"QYG_SSH_AUTHORIZED_KEYS_FILE"`
	SshHost               string `env:"QYG_SSH_HOST"`
	SshPort               string `env:"QYG_SSH_PORT"`
	SshExternalDomain     string `env:"QYG_SSH_EXTERNAL_DOMAIN"`
	SshUsername           string `env:"QYG_SSH_USERNAME"`

	GithubClientKey string `env:"QYG_GITHUB_CLIENT_KEY"`
	GithubSecret    string `env:"QYG_GITHUB_SECRET"`
	GithubName      string `env:"QYG_GITHUB_NAME"`

	GitlabClientKey string `env:"QYG_GITLAB_CLIENT_KEY"`
	GitlabSecret    string `env:"QYG_GITLAB_SECRET"`
	GitlabUrl       string `env:"QYG_GITLAB_URL"`
	GitlabName      string `env:"QYG_GITLAB_NAME"`

	GiteaClientKey string `env:"QYG_GITEA_CLIENT_KEY"`
	GiteaSecret    string `env:"QYG_GITEA_SECRET"`
	GiteaUrl       string `env:"QYG_GITEA_URL"`
	GiteaName      string `env:"QYG_GITEA_NAME"`

	OIDCProviderName   string `env:"QYG_OIDC_PROVIDER_NAME"`
	OIDCClientKey      string `env:"QYG_OIDC_CLIENT_KEY"`
	OIDCSecret         string `env:"QYG_OIDC_SECRET"`
	OIDCDiscoveryUrl   string `env:"QYG_OIDC_DISCOVERY_URL"`
	OIDCGroupClaimName string `env:"QYG_OIDC_GROUP_CLAIM_NAME"`
	OIDCAdminGroup     string `env:"QYG_OIDC_ADMIN_GROUP"`

	GoogleName      string `env:"QYG_GOOGLE_NAME"`
	GoogleClientKey string `env:"QYG_GOOGLE_CLIENT_KEY"`
	GoogleSecret    string `env:"QYG_GOOGLE_SECRET"`

	// Generic OAuth 2.0 provider: any authorization server that exposes a
	// JSON userinfo endpoint.
	OAuth2Name        string `env:"QYG_OAUTH2_NAME"`
	OAuth2ClientKey   string `env:"QYG_OAUTH2_CLIENT_KEY"`
	OAuth2Secret      string `env:"QYG_OAUTH2_SECRET"`
	OAuth2AuthUrl     string `env:"QYG_OAUTH2_AUTH_URL"`
	OAuth2TokenUrl    string `env:"QYG_OAUTH2_TOKEN_URL"`
	OAuth2UserinfoUrl string `env:"QYG_OAUTH2_USERINFO_URL"`
	OAuth2Scopes      string `env:"QYG_OAUTH2_SCOPES"`
	OAuth2IdField     string `env:"QYG_OAUTH2_ID_FIELD"`
	OAuth2NameField   string `env:"QYG_OAUTH2_NAME_FIELD"`
	OAuth2EmailField  string `env:"QYG_OAUTH2_EMAIL_FIELD"`

	// Per-provider switches. They are panel-only (no environment variable):
	// the admin panel writes them into the database and applies them here.
	// A provider is offered when its switch is on and its credentials exist.
	GithubEnabled bool
	GoogleEnabled bool
	GitlabEnabled bool
	GiteaEnabled  bool
	OIDCEnabled   bool
	OAuth2Enabled bool

	MetricsEnabled bool   `env:"QYG_METRICS_ENABLED"`
	MetricsHost    string `env:"QYG_METRICS_HOST"`
	MetricsPort    string `env:"QYG_METRICS_PORT"`

	LDAPUrl             string `env:"QYG_LDAP_URL"`
	LDAPBindDn          string `env:"QYG_LDAP_BIND_DN"`
	LDAPBindCredentials string `env:"QYG_LDAP_BIND_CREDENTIALS"`
	LDAPSearchBase      string `env:"QYG_LDAP_SEARCH_BASE"`
	LDAPSearchFilter    string `env:"QYG_LDAP_SEARCH_FILTER"`

	CustomName    string       `env:"QYG_CUSTOM_NAME"`
	CustomLogo    string       `env:"QYG_CUSTOM_LOGO"`
	CustomFavicon string       `env:"QYG_CUSTOM_FAVICON"`
	StaticLinks   []StaticLink `env:"QYG_CUSTOM_STATIC_LINK"`
}

type StaticLink struct {
	Name string `env:"QYG_CUSTOM_STATIC_LINK_#_NAME"`
	Path string `env:"QYG_CUSTOM_STATIC_LINK_#_PATH"`
}

// SshEnabled reports whether SSH git access is offered in any form.
func (c *config) SshEnabled() bool {
	return c.SshGit == SshServerBuiltin || c.SshGit == SshServerHost
}

// SshBuiltin reports whether QingyuGist runs its own embedded SSH server.
func (c *config) SshBuiltin() bool {
	return c.SshGit == SshServerBuiltin
}

// SshManagesAuthorizedKeys reports whether QingyuGist maintains an authorized_keys
// file: host mode with a configured file path.
func (c *config) SshManagesAuthorizedKeys() bool {
	return c.SshGit == SshServerHost && c.SshAuthorizedKeysFile != ""
}

// A third-party provider is offered only when the admin switch is on and every
// credential it needs is present. The helpers keep that rule in one place:
// handlers, middlewares and templates all ask these instead of poking at the
// individual fields.

func (c *config) GithubOAuthEnabled() bool {
	return c.GithubEnabled && c.GithubClientKey != "" && c.GithubSecret != ""
}

func (c *config) GoogleOAuthEnabled() bool {
	return c.GoogleEnabled && c.GoogleClientKey != "" && c.GoogleSecret != ""
}

func (c *config) GitlabOAuthEnabled() bool {
	return c.GitlabEnabled && c.GitlabClientKey != "" && c.GitlabSecret != ""
}

func (c *config) GiteaOAuthEnabled() bool {
	return c.GiteaEnabled && c.GiteaClientKey != "" && c.GiteaSecret != ""
}

func (c *config) OpenIDConnectEnabled() bool {
	return c.OIDCEnabled && c.OIDCClientKey != "" && c.OIDCSecret != "" && c.OIDCDiscoveryUrl != ""
}

func (c *config) OAuth2OAuthEnabled() bool {
	return c.OAuth2Enabled && c.OAuth2ClientKey != "" && c.OAuth2Secret != "" &&
		c.OAuth2AuthUrl != "" && c.OAuth2TokenUrl != "" && c.OAuth2UserinfoUrl != ""
}

// OAuthAnyEnabled reports whether the login page should offer third-party sign-in.
func (c *config) OAuthAnyEnabled() bool {
	return c.GithubOAuthEnabled() || c.GoogleOAuthEnabled() || c.GitlabOAuthEnabled() ||
		c.GiteaOAuthEnabled() || c.OpenIDConnectEnabled() || c.OAuth2OAuthEnabled()
}

func configWithDefaults() (*config, error) {
	c := &config{}

	c.SecretKey = ""

	c.LogLevel = "warn"
	c.LogOutput = "stdout,file"
	c.QingyuGistHome = ""
	c.DBUri = "qingyugist.db"
	c.Index = "bleve"
	c.SearchDefault = "content"

	c.SqliteJournalMode = "WAL"

	c.HttpHost = "0.0.0.0"
	c.HttpPort = "6157"
	c.HttpGit = true

	c.ApiEnabled = true

	c.CommentsEnabled = true
	c.CommentNotify = true
	c.CommentsAllowGuests = false

	c.UnixSocketPermissions = "0666"

	c.SshGit = SshServerBuiltin
	c.SshHost = "0.0.0.0"
	c.SshPort = "2222"

	c.GitlabName = "GitLab"
	c.GithubName = "GitHub"

	c.GiteaUrl = "https://gitea.com"
	c.GiteaName = "Gitea"

	c.GoogleName = "Google"

	c.OAuth2Name = "OAuth 2.0"
	c.OAuth2Scopes = "openid profile email"
	c.OAuth2IdField = "id"
	c.OAuth2NameField = "username"
	c.OAuth2EmailField = "email"

	c.MetricsEnabled = false
	c.MetricsHost = "0.0.0.0"
	c.MetricsPort = "6158"

	return c, nil
}

// InitConfig resolves the runtime configuration from the compiled-in defaults
// and the QYG_* environment variables. There is no configuration file: every
// option either has a default or is set through the environment.
func InitConfig(out io.Writer) error {
	// Default values
	c, err := configWithDefaults()
	if err != nil {
		return err
	}

	if err = loadConfigFromEnv(c, out); err != nil {
		return err
	}

	// ssh.git-enabled accepts the explicit modes (builtin, host, disabled) as well
	// as the legacy booleans (true → builtin, false → disabled). Collapse whatever
	// was provided into a canonical mode.
	c.SshGit = normalizeSshGitMode(c.SshGit)

	// Git hooks run as a child of the server, which exports QYG_HOME_INTERNAL
	// for exactly this purpose, so honour it when QYG_HOME is absent.
	if c.QingyuGistHome == "" {
		if internalHome, ok := envLookup("QYG_HOME_INTERNAL"); ok && internalHome != "" {
			c.QingyuGistHome = internalHome
		}
	}

	if c.QingyuGistHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("qingyugist home directory is not set and current user home directory could not be determined; please set the QYG_HOME environment variable")
		}

		c.QingyuGistHome = filepath.Join(homeDir, ".qingyugist")
		if !pathExists(c.QingyuGistHome) {
			legacyHome := filepath.Join(homeDir, "."+legacyName)
			if pathExists(legacyHome) {
				c.QingyuGistHome = legacyHome
				_, _ = fmt.Fprintf(out, "Using existing data directory %s; move it to %s when convenient.\n", legacyHome, filepath.Join(homeDir, ".qingyugist"))
			}
		}
	}

	if err = checks(c); err != nil {
		return err
	}

	// A provider whose credentials came from the environment starts enabled, so
	// upgrades keep every existing sign-in button. The admin panel can switch it
	// off afterwards; its choice lands in the database and wins over this default.
	c.GithubEnabled = c.GithubClientKey != "" && c.GithubSecret != ""
	c.GoogleEnabled = c.GoogleClientKey != "" && c.GoogleSecret != ""
	c.GitlabEnabled = c.GitlabClientKey != "" && c.GitlabSecret != ""
	c.GiteaEnabled = c.GiteaClientKey != "" && c.GiteaSecret != ""
	c.OIDCEnabled = c.OIDCClientKey != "" && c.OIDCSecret != "" && c.OIDCDiscoveryUrl != ""
	c.OAuth2Enabled = c.OAuth2ClientKey != "" && c.OAuth2Secret != "" &&
		c.OAuth2AuthUrl != "" && c.OAuth2TokenUrl != "" && c.OAuth2UserinfoUrl != ""

	C = c

	if c.LogPath == "" {
		c.LogPath = filepath.Join(GetHomeDir(), "log")
	}

	migrateLegacyData(out)

	if err = migrateConfig(); err != nil {
		return err
	}

	if err = os.Setenv("QYG_HOME_INTERNAL", GetHomeDir()); err != nil {
		return err
	}
	ExportLegacyEnv()
	return nil
}

func InitLog() {
	if err := os.MkdirAll(C.LogPath, 0755); err != nil {
		panic(err)
	}

	var level zerolog.Level
	level, err := zerolog.ParseLevel(C.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	var logWriters []io.Writer
	logOutputTypes := strings.Split(strings.ToLower(C.LogOutput), ",")
	slices.Sort(logOutputTypes)
	logOutputTypes = slices.Compact(logOutputTypes)

	consoleWriter := zerolog.NewConsoleWriter(
		func(w *zerolog.ConsoleWriter) {
			w.TimeFormat = time.TimeOnly
			w.FormatCaller = func(i interface{}) string {
				file := i.(string)
				index := strings.Index(file, "internal")
				if index == -1 {
					return file
				}
				return file[index:]
			}
		},
	)

	for _, logOutputType := range logOutputTypes {
		logOutputType = strings.TrimSpace(logOutputType)
		if !slices.Contains([]string{"stdout", "file"}, logOutputType) {
			defer func() { log.Warn().Msg("Invalid log output type: " + logOutputType) }()
			continue
		}

		switch logOutputType {
		case "stdout":
			logWriters = append(logWriters, consoleWriter)
			defer func() { log.Debug().Msg("Logging to stdout") }()
		case "file":
			file, err := os.OpenFile(filepath.Join(C.LogPath, "qingyugist.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				panic(err)
			}
			logWriters = append(logWriters, file)
			defer func() { log.Debug().Msg("Logging to file: " + file.Name()) }()
		}
	}
	if len(logWriters) == 0 {
		logWriters = append(logWriters, consoleWriter)
		defer func() { log.Warn().Msg("No valid log outputs, defaulting to stdout") }()
	}

	multi := zerolog.MultiLevelWriter(logWriters...)
	log.Logger = zerolog.New(multi).Level(level).With().Caller().Timestamp().Logger()

	if !slices.Contains([]string{"debug", "info", "warn", "error", "fatal"}, strings.ToLower(C.LogLevel)) {
		log.Warn().Msg("Invalid log level: " + C.LogLevel)
	}
}

// gitVersionRegex extracts the major and minor numbers from a git version string.
// It tolerates a leading "git version " prefix as well as suffixes such as the
// ".windows.1" appended by Git for Windows (e.g. "git version 2.50.1.windows.1").
var gitVersionRegex = regexp.MustCompile(`(\d+)\.(\d+)`)

func CheckGitVersion(version string) (bool, error) {
	matches := gitVersionRegex.FindStringSubmatch(version)
	if matches == nil {
		return false, fmt.Errorf("invalid version string: %q", version)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return false, fmt.Errorf("invalid major version number")
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return false, fmt.Errorf("invalid minor version number")
	}

	// Check if version is prior to 2.28
	if major < 2 || (major == 2 && minor < 28) {
		return false, nil
	}
	return true, nil
}

func GetHomeDir() string {
	absolutePath, _ := filepath.Abs(C.QingyuGistHome)
	return filepath.Clean(absolutePath)
}

func SetupSecretKey() {
	if C.SecretKey == "" {
		path := filepath.Join(GetHomeDir(), "qingyugist-secret.key")
		SecretKey, _ = session.GenerateSecretKey(path)
	} else {
		SecretKey = []byte(C.SecretKey)
	}
}

func loadConfigFromEnv(c *config, out io.Writer) error {
	v := reflect.ValueOf(c).Elem()
	var envVars []string

	for i := 0; i < v.NumField(); i++ {
		tag := v.Type().Field(i).Tag.Get("env")

		if tag == "" {
			continue
		}

		envValue, present := envLookup(strings.ToUpper(tag))
		if !present {
			envValue = ""
		}
		if envValue == "" && v.Field(i).Kind() != reflect.Slice {
			continue
		}

		switch v.Field(i).Kind() {
		case reflect.String:
			v.Field(i).SetString(envValue)
			envVars = append(envVars, tag)
		case reflect.Bool:
			boolVal, err := strconv.ParseBool(envValue)
			if err != nil {
				return err
			}
			v.Field(i).SetBool(boolVal)
			envVars = append(envVars, tag)
		case reflect.Slice:
			if v.Type().Field(i).Type.Elem().Kind() == reflect.Struct {
				prefix := strings.ToUpper(tag) + "_"
				var sliceValue reflect.Value
				elemType := v.Type().Field(i).Type.Elem()

				for index := 0; ; index++ {
					allFieldsPresent := true
					elemValue := reflect.New(elemType).Elem()

					for j := 0; j < elemValue.NumField() && allFieldsPresent; j++ {
						elemField := elemValue.Type().Field(j)
						envName := fmt.Sprintf("%s%d_%s", prefix, index, strings.ToUpper(elemField.Name))
						envValue, present := envLookup(envName)

						if !present {
							allFieldsPresent = false
							break
						}

						envVars = append(envVars, envName)
						elemValue.Field(j).SetString(envValue)
					}

					if !allFieldsPresent {
						break
					}

					if sliceValue.Kind() != reflect.Slice {
						sliceValue = reflect.MakeSlice(v.Type().Field(i).Type, 0, index+1)
					}
					sliceValue = reflect.Append(sliceValue, elemValue)
				}

				if sliceValue.IsValid() {
					v.Field(i).Set(sliceValue)
				}
			}
		default:
			return fmt.Errorf("unsupported type: %s", v.Field(i).Kind())
		}

	}

	if len(envVars) > 0 {
		_, _ = fmt.Fprintln(out, "Using environment variables config: "+strings.Join(envVars, ", "))
	} else {
		_, _ = fmt.Fprintln(out, "No environment variables config specified.")
	}

	return nil
}

// normalizeSshGitMode maps every accepted ssh.git-enabled value to a canonical
// mode. The legacy booleans are preserved for backward compatibility (true →
// builtin, false → disabled); unknown values are returned untouched so checks()
// can reject them.
func normalizeSshGitMode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "host":
		return SshServerHost
	case "false", "f", "0", "no", "off", "disabled":
		return SshServerDisabled
	case "true", "t", "1", "yes", "on", "builtin", "":
		return SshServerBuiltin
	default:
		return strings.TrimSpace(s)
	}
}

func checks(c *config) error {
	switch c.SshGit {
	case SshServerBuiltin, SshServerHost, SshServerDisabled:
	default:
		return fmt.Errorf("invalid ssh.git-enabled %q (must be %q, %q or %q, or a boolean)", c.SshGit, SshServerBuiltin, SshServerHost, SshServerDisabled)
	}

	if _, err := url.Parse(c.ExternalUrl); err != nil {
		return err
	}

	if _, err := url.Parse(c.GiteaUrl); err != nil {
		return err
	}

	if _, err := url.Parse(c.OIDCDiscoveryUrl); err != nil {
		return err
	}

	if c.LDAPUrl != "" && c.LDAPBindDn != "" && c.LDAPBindCredentials == "" {
		return fmt.Errorf("ldap.bind-dn is set but ldap.bind-credentials is empty; " +
			"set the password, or leave ldap.bind-dn empty to query the LDAP server anonymously")
	}

	return nil
}
