package oauth

import (
	gocontext "context"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/google"
	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/web/context"
)

type GoogleProvider struct {
	Provider
	URL string
}

func (p *GoogleProvider) RegisterProvider() error {
	goth.UseProviders(
		google.New(
			config.C.GoogleClientKey,
			config.C.GoogleSecret,
			urlJoin(p.URL, "/oauth/google/callback"),
			"openid", "email", "profile",
		),
	)

	return nil
}

func (p *GoogleProvider) BeginAuthHandler(ctx *context.Context) {
	ctxValue := gocontext.WithValue(ctx.Request().Context(), gothic.ProviderParamKey, GoogleProviderString)
	ctx.SetRequest(ctx.Request().WithContext(ctxValue))

	gothic.BeginAuthHandler(ctx.Response(), ctx.Request())
}

func (p *GoogleProvider) UserHasProvider(user *db.User) bool {
	return user.GoogleID != ""
}

func NewGoogleProvider(url string) *GoogleProvider {
	return &GoogleProvider{URL: url}
}

type GoogleCallbackProvider struct {
	CallbackProvider
	User *goth.User
}

func (p *GoogleCallbackProvider) GetProvider() string {
	return GoogleProviderString
}

func (p *GoogleCallbackProvider) GetProviderUser() *goth.User {
	return p.User
}

func (p *GoogleCallbackProvider) GetProviderUserID(user *db.User) bool {
	return user.GoogleID != ""
}

// Google does not publish SSH keys, so users sign in here for the web ui only.
func (p *GoogleCallbackProvider) GetProviderUserSSHKeys() ([]string, error) {
	return nil, nil
}

func (p *GoogleCallbackProvider) UpdateUserDB(user *db.User) {
	user.GoogleID = p.User.UserID
	syncAvatarHash(user, p.User.Email)
}

// IsAdmin is false: Google groups are not read, so it never grants admin.
func (p *GoogleCallbackProvider) IsAdmin() bool {
	return false
}

func NewGoogleCallbackProvider(user *goth.User) CallbackProvider {
	return &GoogleCallbackProvider{User: user}
}
