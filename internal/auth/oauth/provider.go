package oauth

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/web/context"
	"github.com/rs/zerolog/log"
)

const (
	GitHubProviderString = "github"
	GoogleProviderString = "google"
	GitLabProviderString = "gitlab"
	GiteaProviderString  = "gitea"
	OpenIDConnectString  = "openid-connect"
	OAuth2ProviderString = "oauth2"
)

type Provider interface {
	RegisterProvider() error
	BeginAuthHandler(ctx *context.Context)
	UserHasProvider(user *db.User) bool
}

type CallbackProvider interface {
	GetProvider() string
	GetProviderUser() *goth.User
	GetProviderUserID(user *db.User) bool
	GetProviderUserSSHKeys() ([]string, error)
	UpdateUserDB(user *db.User)
	IsAdmin() bool
}

func ProviderEnabled(provider string) bool {
	switch provider {
	case GitHubProviderString:
		return config.C.GithubOAuthEnabled()
	case GoogleProviderString:
		return config.C.GoogleOAuthEnabled()
	case GitLabProviderString:
		return config.C.GitlabOAuthEnabled()
	case GiteaProviderString:
		return config.C.GiteaOAuthEnabled()
	case OpenIDConnectString:
		return config.C.OpenIDConnectEnabled()
	case OAuth2ProviderString:
		return config.C.OAuth2OAuthEnabled()
	}

	return false
}

func DefineProvider(provider string, url string) (Provider, error) {
	switch provider {
	case GitHubProviderString:
		return NewGitHubProvider(url), nil
	case GoogleProviderString:
		return NewGoogleProvider(url), nil
	case GitLabProviderString:
		return NewGitLabProvider(url), nil
	case GiteaProviderString:
		return NewGiteaProvider(url), nil
	case OpenIDConnectString:
		return NewOIDCProvider(url), nil
	case OAuth2ProviderString:
		return NewOAuth2Provider(url), nil
	}

	return nil, fmt.Errorf("unsupported provider %s", provider)
}

func CompleteUserAuth(ctx *context.Context) (CallbackProvider, error) {
	injectCodeVerifier(ctx)

	user, err := gothic.CompleteUserAuth(ctx.Response(), ctx.Request())
	if err != nil {
		return nil, err
	}

	switch user.Provider {
	case GitHubProviderString:
		return NewGitHubCallbackProvider(&user), nil
	case GoogleProviderString:
		return NewGoogleCallbackProvider(&user), nil
	case GitLabProviderString:
		return NewGitLabCallbackProvider(&user), nil
	case GiteaProviderString:
		return NewGiteaCallbackProvider(&user), nil
	case OpenIDConnectString:
		return NewOIDCCallbackProvider(&user), nil
	case OAuth2ProviderString:
		return NewOAuth2CallbackProvider(&user), nil
	}

	return nil, fmt.Errorf("unsupported provider %s", user.Provider)
}

func NewCallbackProviderFromSession(provider string, userID string, nickname string, email string) (CallbackProvider, error) {
	user := &goth.User{
		Provider: provider,
		UserID:   userID,
		NickName: nickname,
		Email:    email,
	}

	switch provider {
	case GitHubProviderString:
		return NewGitHubCallbackProvider(user), nil
	case GoogleProviderString:
		return NewGoogleCallbackProvider(user), nil
	case GitLabProviderString:
		return NewGitLabCallbackProvider(user), nil
	case GiteaProviderString:
		return NewGiteaCallbackProvider(user), nil
	case OpenIDConnectString:
		return NewOIDCCallbackProvider(user), nil
	case OAuth2ProviderString:
		return NewOAuth2CallbackProvider(user), nil
	}

	return nil, fmt.Errorf("unsupported provider %s", provider)
}

// ProviderDisplayName returns the name the admin panel gave a provider, so the
// ui says "Sign in with <name>" instead of the protocol behind it.
func ProviderDisplayName(provider string) string {
	switch provider {
	case GitHubProviderString:
		return nameOrDefault(config.C.GithubName, "GitHub")
	case GoogleProviderString:
		return nameOrDefault(config.C.GoogleName, "Google")
	case GitLabProviderString:
		return nameOrDefault(config.C.GitlabName, "GitLab")
	case GiteaProviderString:
		return nameOrDefault(config.C.GiteaName, "Gitea")
	case OpenIDConnectString:
		return nameOrDefault(config.C.OIDCProviderName, "OpenID")
	case OAuth2ProviderString:
		return nameOrDefault(config.C.OAuth2Name, "OAuth 2.0")
	}

	return provider
}

func nameOrDefault(name, fallback string) string {
	if strings.TrimSpace(name) == "" {
		return fallback
	}
	return strings.TrimSpace(name)
}

// syncAvatarHash records the Gravatar key for an account's email address.
//
// Providers all hand out their own picture URLs (avatars.githubusercontent.com,
// Google's CDN, …): they are slow from outside their region, they turn every
// page load into a third-party request, and the account holder cannot choose
// what they see. So the picture is always derived from the address instead —
// see settings.AvatarURL — and only the hash is stored.
func syncAvatarHash(user *db.User, email string) {
	// The account's own address wins: it is what the site displays and what the
	// settings page changes. The provider address only fills the gap when the
	// account has none, which is the case for a locally registered account that
	// has just linked a provider for the first time.
	if strings.TrimSpace(user.Email) == "" {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			user.MD5Hash = ""
			return
		}
		user.Email = email
	} else {
		email = user.Email
	}

	user.MD5Hash = fmt.Sprintf("%x", md5.Sum([]byte(strings.ToLower(strings.TrimSpace(email)))))
}

func urlJoin(base string, elem ...string) string {
	joined, err := url.JoinPath(base, elem...)
	if err != nil {
		log.Error().Err(err).Msg("Cannot join url")
	}

	return joined
}

func readKeys(response *http.Response) ([]string, error) {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("could not get user keys %v", err)
	}

	keys := strings.Split(string(body), "\n")
	if len(keys[len(keys)-1]) == 0 {
		keys = keys[:len(keys)-1]
	}

	return keys, nil
}
