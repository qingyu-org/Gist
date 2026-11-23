package auth

import (
	"crypto/md5"
	"errors"
	"fmt"
	"strings"

	"github.com/gorilla/sessions"
	"github.com/qingyu-org/Gist/internal/auth/oauth"
	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/i18n"
	"github.com/qingyu-org/Gist/internal/validator"
	"github.com/qingyu-org/Gist/internal/web/context"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const oauthInvitationSessionKey = "oauthInvitation"

// sessionProvider is the provider string stored while the oauth registration
// form is open, or "" when the session has already been cleaned up.
func sessionProvider(sess *sessions.Session) string {
	provider, _ := sess.Values["oauthProvider"].(string)
	return provider
}

// allowed is false after a rejection, even when writing the redirect succeeds.
func allowOauthSignup(ctx *context.Context) (*db.Invitation, bool, error) {
	invitationID, ok := ctx.GetSession().Values[oauthInvitationSessionKey].(uint)
	if ok {
		invitation, err := db.GetInvitationByID(invitationID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ctx.ErrorRes(500, "Cannot check for invitation code", err)
		}
		if err == nil && invitation.IsUsable() {
			return invitation, true, nil
		}
	}

	if ctx.GetData("DisableSignup") == true {
		ctx.AddFlash(ctx.Tr("error.signup-disabled"), "error")
		return nil, false, ctx.Redirect(302, "/login")
	}
	return nil, true, nil
}

func Oauth(ctx *context.Context) error {
	providerStr := ctx.Param("provider")

	// A provider switched off in the admin panel must not start a login, even
	// when its credentials are still stored or present in the environment.
	if !oauth.ProviderEnabled(providerStr) {
		ctx.AddFlash(ctx.Tr("error.oauth-unsupported"), "error")
		return ctx.Redirect(302, "/login")
	}

	httpProtocol := "http"
	if ctx.Request().TLS != nil || ctx.Request().Header.Get("X-Forwarded-Proto") == "https" {
		httpProtocol = "https"
	}

	forwarded_hdr := ctx.Request().Header.Get("Forwarded")
	if forwarded_hdr != "" {
		fields := strings.Split(forwarded_hdr, ";")
		fwd := make(map[string]string)
		for _, v := range fields {
			p := strings.Split(v, "=")
			fwd[p[0]] = p[1]
		}
		val, ok := fwd["proto"]
		if ok && val == "https" {
			httpProtocol = "https"
		}
	}

	var qingyugistURL string
	if config.C.ExternalUrl != "" {
		qingyugistURL = config.C.ExternalUrl
	} else {
		qingyugistURL = httpProtocol + "://" + ctx.Request().Host
	}

	provider, err := oauth.DefineProvider(providerStr, qingyugistURL)
	if err != nil {
		ctx.AddFlash(ctx.Tr("error.oauth-unsupported"), "error")
		return ctx.Redirect(302, "/login")
	}

	if err = provider.RegisterProvider(); err != nil {
		return ctx.ErrorRes(500, "Cannot create provider", err)
	}

	provider.BeginAuthHandler(ctx)
	return nil
}

func OauthCallback(ctx *context.Context) error {
	provider, err := oauth.CompleteUserAuth(ctx)
	if err != nil {
		ctx.AddFlash(fmt.Sprintf("%s: %s", ctx.Tr("auth.oauth.no-provider"), err.Error()), "error")
		return ctx.Redirect(302, "/login")
	}

	currUser := ctx.User
	user := provider.GetProviderUser()

	// if user is logged in, link account to user and update its avatar URL
	if currUser != nil {
		// check if this OAuth account is already linked to another user
		if existingUser, err := db.GetUserByProvider(user.UserID, provider.GetProvider()); err == nil && existingUser != nil {
			if existingUser.ID != currUser.ID {
				ctx.AddFlash(ctx.Tr("flash.auth.oauth-already-linked", oauth.ProviderDisplayName(provider.GetProvider())), "error")
				return ctx.RedirectTo("/settings")
			}

			// the account is already linked to the user currently logged in:
			// this is a re-authentication, not a link attempt
			return ctx.RedirectTo("/")
		}

		provider.UpdateUserDB(currUser)

		if err = currUser.Update(); err != nil {
			return ctx.ErrorRes(500, "Cannot update user "+oauth.ProviderDisplayName(provider.GetProvider())+" id", err)
		}

		ctx.AddFlash(ctx.Tr("flash.auth.account-linked-oauth", oauth.ProviderDisplayName(provider.GetProvider())), "success")
		return ctx.RedirectTo("/settings")
	}

	userDB, err := db.GetUserByProvider(user.UserID, provider.GetProvider())
	// if user is not in database, redirect to OAuth registration page
	if err != nil {
		if _, allowed, err := allowOauthSignup(ctx); err != nil || !allowed {
			return err
		}

		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return ctx.ErrorRes(500, "Cannot get user", err)
		}

		if user.NickName == "" {
			user.NickName = strings.Split(user.Email, "@")[0]
		}

		sess := ctx.GetSession()
		sess.Values["oauthProvider"] = provider.GetProvider()
		sess.Values["oauthUserID"] = user.UserID
		sess.Values["oauthNickname"] = user.NickName
		sess.Values["oauthEmail"] = user.Email
		sess.Values["oauthIsAdmin"] = provider.IsAdmin()

		sess.Options.MaxAge = 10 * 60 // 10 minutes
		ctx.SaveSession(sess)

		return ctx.RedirectTo("/oauth/register")
	}

	// promote user to admin from oidc group
	if !userDB.IsAdmin && provider.IsAdmin() {
		userDB.IsAdmin = true
		if err = userDB.Update(); err != nil {
			return ctx.ErrorRes(500, "Cannot set user admin", err)
		}
	}

	// The password login path sends accounts with a second factor through /mfa;
	// a provider login must do the same, otherwise linking an OAuth account
	// silently downgrades the account to a single factor.
	var hasWebauthn, hasTotp bool
	if hasWebauthn, hasTotp, err = userDB.HasMFA(); err != nil {
		return ctx.ErrorRes(500, "Cannot check for user MFA", err)
	}

	sess := ctx.GetSession()
	if hasWebauthn || hasTotp {
		sess.Values["mfaID"] = userDB.ID
		sess.Options.MaxAge = 5 * 60 // 5 minutes
		ctx.SaveSession(sess)
		return ctx.RedirectTo("/mfa")
	}

	sess.Values["user"] = userDB.ID
	sess.Options.MaxAge = 60 * 60 * 24 * 365 // 1 year
	sess.Values["sessionVersion"] = userDB.SessionVersion
	ctx.SaveSession(sess)
	ctx.DeleteCsrfCookie()

	return ctx.RedirectTo("/")
}

func OauthRegister(ctx *context.Context) error {
	if _, allowed, err := allowOauthSignup(ctx); err != nil || !allowed {
		return err
	}

	sess := ctx.GetSession()

	ctx.SetData("title", ctx.TrH("auth.oauth.complete-registration"))
	ctx.SetData("htmlTitle", ctx.TrH("auth.oauth.complete-registration"))
	ctx.SetData("oauthProvider", oauth.ProviderDisplayName(sessionProvider(sess)))
	ctx.SetData("oauthNickname", sess.Values["oauthNickname"])
	ctx.SetData("oauthEmail", sess.Values["oauthEmail"])

	return ctx.Html("oauth_register.html")
}

func ProcessOauthRegister(ctx *context.Context) error {
	invitation, allowed, err := allowOauthSignup(ctx)
	if err != nil || !allowed {
		return err
	}

	sess := ctx.GetSession()

	providerStr := sess.Values["oauthProvider"].(string)
	oauthUserID := sess.Values["oauthUserID"].(string)

	setOauthRegisterData := func(dto *db.OAuthRegisterDTO) {
		ctx.SetData("title", ctx.TrH("auth.oauth.complete-registration"))
		ctx.SetData("htmlTitle", ctx.TrH("auth.oauth.complete-registration"))
		ctx.SetData("oauthProvider", oauth.ProviderDisplayName(providerStr))
		if dto != nil {
			ctx.SetData("oauthNickname", dto.Username)
			ctx.SetData("oauthEmail", dto.Email)
		} else {
			ctx.SetData("oauthNickname", sess.Values["oauthNickname"])
			ctx.SetData("oauthEmail", sess.Values["oauthEmail"])
		}
	}

	// Bind and validate form data
	dto := new(db.OAuthRegisterDTO)
	if err := ctx.Bind(dto); err != nil {
		return ctx.ErrorRes(400, ctx.Tr("error.cannot-bind-data"), err)
	}

	if err := ctx.Validate(dto); err != nil {
		ctx.AddFlash(validator.ValidationMessages(&err, ctx.GetData("locale").(*i18n.Locale)), "error")
		setOauthRegisterData(dto)
		return ctx.Html("oauth_register.html")
	}

	if exists, err := db.UserExists(dto.Username); err != nil || exists {
		ctx.AddFlash(ctx.Tr("flash.auth.username-exists"), "error")
		setOauthRegisterData(dto)
		return ctx.Html("oauth_register.html")
	}

	// Check if OAuth account is already linked to another user (race condition protection)
	if existingUser, err := db.GetUserByProvider(oauthUserID, providerStr); err == nil && existingUser != nil {
		ctx.AddFlash(ctx.Tr("flash.auth.oauth-already-linked", oauth.ProviderDisplayName(providerStr)), "error")
		setOauthRegisterData(dto)
		return ctx.Html("oauth_register.html")
	}

	userDB := &db.User{
		Username: dto.Username,
		Email:    dto.Email,
	}
	if dto.Email != "" {
		userDB.MD5Hash = fmt.Sprintf("%x", md5.Sum([]byte(strings.ToLower(strings.TrimSpace(dto.Email)))))
	}

	nickname := ""
	if n, ok := sess.Values["oauthNickname"].(string); ok {
		nickname = n
	}
	callbackProvider, err := oauth.NewCallbackProviderFromSession(providerStr, oauthUserID, nickname, dto.Email)
	if err != nil {
		return ctx.ErrorRes(500, "Cannot create provider", err)
	}
	callbackProvider.UpdateUserDB(userDB)

	if err := userDB.Create(); err != nil {
		if db.IsUniqueConstraintViolation(err) {
			ctx.AddFlash(ctx.Tr("flash.auth.username-exists"), "error")
			setOauthRegisterData(dto)
			return ctx.Html("oauth_register.html")
		}
		return ctx.ErrorRes(500, "Cannot create user", err)
	}

	if config.C.OIDCAdminGroup == "" && userDB.ID == 1 {
		if err := userDB.SetAdmin(); err != nil {
			return ctx.ErrorRes(500, "Cannot set user admin", err)
		}
	}

	if isAdmin, ok := sess.Values["oauthIsAdmin"].(bool); ok && isAdmin {
		userDB.IsAdmin = true
		_ = userDB.Update()
	}
	if invitation != nil {
		if err := invitation.Use(); err != nil {
			return ctx.ErrorRes(500, "Cannot use invitation", err)
		}
	}

	keys, err := callbackProvider.GetProviderUserSSHKeys()
	if err != nil {
		ctx.AddFlash(ctx.Tr("flash.auth.user-sshkeys-not-retrievable"), "warning")
		log.Error().Err(err).Msg("Could not get user keys")
	} else {
		for _, key := range keys {
			sshKey := db.SSHKey{
				Title:   "Added from " + providerStr,
				Content: key,
				User:    *userDB,
			}
			if err = sshKey.Create(); err != nil {
				ctx.AddFlash(ctx.Tr("flash.auth.user-sshkeys-not-created"), "error")
				log.Error().Err(err).Msg("Could not create ssh key")
			}
		}
	}

	delete(sess.Values, "oauthProvider")
	delete(sess.Values, "oauthUserID")
	delete(sess.Values, "oauthNickname")
	delete(sess.Values, "oauthEmail")
	delete(sess.Values, "oauthIsAdmin")
	delete(sess.Values, oauthInvitationSessionKey)

	sess.Values["user"] = userDB.ID
	sess.Values["sessionVersion"] = userDB.SessionVersion
	sess.Options.MaxAge = 60 * 60 * 24 * 365 // 1 year
	ctx.SaveSession(sess)
	ctx.DeleteCsrfCookie()

	return ctx.RedirectTo("/")
}

func OauthUnlink(ctx *context.Context) error {
	providerStr := ctx.Param("provider")
	provider, err := oauth.DefineProvider(ctx.Param("provider"), "")
	if err != nil {
		return ctx.ErrorRes(400, ctx.Tr("error.oauth-unsupported"), nil)
	}

	currUser := ctx.User

	if provider.UserHasProvider(currUser) {
		canLogin, err := hasOtherLoginMethod(ctx, currUser, providerStr)
		if err != nil {
			return ctx.ErrorRes(500, "Cannot check for user login methods", err)
		}
		if !canLogin {
			ctx.AddFlash(ctx.Tr("flash.auth.oauth-unlink-only-login-method"), "error")
			return ctx.RedirectTo("/settings/authentication")
		}

		if err := currUser.DeleteProviderID(providerStr); err != nil {
			return ctx.ErrorRes(500, "Cannot unlink account from "+oauth.ProviderDisplayName(providerStr), err)
		}

		ctx.AddFlash(ctx.Tr("flash.auth.account-unlinked-oauth", oauth.ProviderDisplayName(providerStr)), "success")
		return ctx.RedirectTo("/settings")
	}

	return ctx.RedirectTo("/settings")
}

// hasOtherLoginMethod reports whether the user can still log in once the given
// provider is unlinked: through another linked provider configured on this
// instance, or through the login form with a password or a passkey. LDAP is not
// counted, as a user without a password may have no directory entry.
func hasOtherLoginMethod(ctx *context.Context, user *db.User, provider string) (bool, error) {
	linked := map[string]bool{
		oauth.GitHubProviderString: user.GithubID != "" && ctx.GetData("githubOauth") == true,
		oauth.GitLabProviderString: user.GitlabID != "" && ctx.GetData("gitlabOauth") == true,
		oauth.GiteaProviderString:  user.GiteaID != "" && ctx.GetData("giteaOauth") == true,
		oauth.OpenIDConnectString:  user.OIDCID != "" && ctx.GetData("oidcOauth") == true,
	}
	for other, usable := range linked {
		if usable && other != provider {
			return true, nil
		}
	}

	if ctx.GetData("DisableLoginForm") == true {
		return false, nil
	}
	if user.Password != "" {
		return true, nil
	}

	hasPasskey, _, err := user.HasMFA()
	return hasPasskey, err
}
