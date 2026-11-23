package auth

import (
	"errors"

	"github.com/qingyu-org/Gist/internal/auth"
	passwordpkg "github.com/qingyu-org/Gist/internal/auth/password"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/i18n"
	"github.com/qingyu-org/Gist/internal/validator"
	"github.com/qingyu-org/Gist/internal/web/context"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func Register(ctx *context.Context) error {
	disableSignup := ctx.GetData("DisableSignup")
	disableForm := ctx.GetData("DisableLoginForm")

	code := ctx.QueryParam("code")
	validInvitation := false
	if code != "" {
		if invitation, err := db.GetInvitationByCode(code); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return ctx.ErrorRes(500, "Cannot check for invitation code", err)
		} else if invitation != nil && invitation.IsUsable() {
			disableSignup = false
			validInvitation = true
			sess := ctx.GetSession()
			sess.Values[oauthInvitationSessionKey] = invitation.ID
			ctx.SaveSession(sess)
		}
	}

	ctx.SetData("title", ctx.TrH("auth.new-account"))
	ctx.SetData("htmlTitle", ctx.TrH("auth.new-account"))
	// In OAuth-only mode the form stays hidden; the third-party buttons are
	// the only way in. A valid invitation code still unlocks the local form.
	if ctx.GetData("OAuthOnlyRegistration") == true && !validInvitation {
		disableForm = true
	}
	ctx.SetData("disableForm", disableForm)
	ctx.SetData("disableSignup", disableSignup)
	ctx.SetData("isLoginPage", false)
	ctx.SetData("authPage", true)
	return ctx.Html("auth_form.html")
}

func ProcessRegister(ctx *context.Context) error {
	disableSignup := ctx.GetData("DisableSignup")

	code := ctx.QueryParam("code")
	invitation, err := db.GetInvitationByCode(code)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return ctx.ErrorRes(500, "Cannot check for invitation code", err)
	} else if invitation.ID != 0 && invitation.IsUsable() {
		disableSignup = false
	}

	if disableSignup == true {
		return ctx.ErrorRes(403, ctx.Tr("error.signup-disabled"), nil)
	}

	if ctx.GetData("DisableLoginForm") == true {
		return ctx.ErrorRes(403, ctx.Tr("error.signup-disabled-form"), nil)
	}

	if ctx.GetData("OAuthOnlyRegistration") == true && invitation.ID == 0 {
		return ctx.ErrorRes(403, ctx.Tr("error.signup-oauth-only"), nil)
	}

	ctx.SetData("title", ctx.TrH("auth.new-account"))
	ctx.SetData("htmlTitle", ctx.TrH("auth.new-account"))

	sess := ctx.GetSession()

	dto := new(db.UserDTO)
	if err := ctx.Bind(dto); err != nil {
		return ctx.ErrorRes(400, ctx.Tr("error.cannot-bind-data"), err)
	}

	if err := ctx.Validate(dto); err != nil {
		ctx.AddFlash(validator.ValidationMessages(&err, ctx.GetData("locale").(*i18n.Locale)), "error")
		ctx.SetData("isLoginPage", false)
		ctx.SetData("authPage", true)
		return ctx.Html("auth_form.html")
	}

	if exists, err := db.UserExists(dto.Username); err != nil || exists {
		ctx.AddFlash(ctx.Tr("flash.auth.username-exists"), "error")
		ctx.SetData("isLoginPage", false)
		ctx.SetData("authPage", true)
		return ctx.Html("auth_form.html")
	}

	user := dto.ToUser()

	password, err := passwordpkg.HashPassword(user.Password)
	if err != nil {
		return ctx.ErrorRes(500, "Cannot hash password", err)
	}
	user.Password = password

	if err = user.Create(); err != nil {
		return ctx.ErrorRes(500, "Cannot create user", err)
	}

	if user.ID == 1 {
		if err = user.SetAdmin(); err != nil {
			return ctx.ErrorRes(500, "Cannot set user admin", err)
		}
	}

	if invitation.ID != 0 {
		if err := invitation.Use(); err != nil {
			return ctx.ErrorRes(500, "Cannot use invitation", err)
		}
	}

	sess.Values["user"] = user.ID
	sess.Values["sessionVersion"] = user.SessionVersion
	ctx.SaveSession(sess)

	return ctx.RedirectTo("/")
}

func Login(ctx *context.Context) error {
	ctx.SetData("title", ctx.TrH("auth.login"))
	ctx.SetData("htmlTitle", ctx.TrH("auth.login"))
	ctx.SetData("disableForm", ctx.GetData("DisableLoginForm"))
	ctx.SetData("isLoginPage", true)
	ctx.SetData("authPage", true)
	return ctx.Html("auth_form.html")
}

func ProcessLogin(ctx *context.Context) error {
	if ctx.GetData("DisableLoginForm") == true {
		return ctx.ErrorRes(403, ctx.Tr("error.login-disabled-form"), nil)
	}

	var user *db.User
	var err error
	sess := ctx.GetSession()

	dto := &db.UserDTO{}
	if err = ctx.Bind(dto); err != nil {
		return ctx.ErrorRes(400, ctx.Tr("error.cannot-bind-data"), err)
	}

	user, err = auth.TryAuthentication(dto.Username, dto.Password)
	if err != nil {
		var authErr auth.AuthError
		if errors.As(err, &authErr) {
			log.Warn().Msg("Invalid HTTP authentication attempt from " + ctx.RealIP())
			ctx.AddFlash(ctx.Tr("flash.auth.invalid-credentials"), "error")
			return ctx.RedirectTo("/login")
		}
		return ctx.ErrorRes(500, "Authentication system error", nil)
	}

	// handle MFA
	var hasWebauthn, hasTotp bool
	if hasWebauthn, hasTotp, err = user.HasMFA(); err != nil {
		return ctx.ErrorRes(500, "Cannot check for user MFA", err)
	}
	if hasWebauthn || hasTotp {
		sess.Values["mfaID"] = user.ID
		sess.Options.MaxAge = 5 * 60 // 5 minutes
		ctx.SaveSession(sess)
		return ctx.RedirectTo("/mfa")
	}

	sess.Values["user"] = user.ID
	sess.Values["sessionVersion"] = user.SessionVersion
	sess.Options.MaxAge = 60 * 60 * 24 * 365 // 1 year
	ctx.SaveSession(sess)
	ctx.DeleteCsrfCookie()

	return ctx.RedirectTo("/")
}

func Logout(ctx *context.Context) error {
	ctx.DeleteSession()
	ctx.DeleteCsrfCookie()
	return ctx.RedirectTo("/explore")
}
