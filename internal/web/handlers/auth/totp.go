package auth

import (
	"net/url"

	passwordpkg "github.com/qingyu-org/Gist/internal/auth/password"
	"github.com/qingyu-org/Gist/internal/auth/totp"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/web/context"
)

// confirmCurrentPassword gates destructive MFA changes on the account password.
// It reports whether the request may proceed; when it may not, the returned
// error is the response to send back. Accounts created through OAuth have no
// password yet, so there is nothing to confirm.
func confirmCurrentPassword(ctx *context.Context, user *db.User) (bool, error) {
	if user.Password == "" {
		return true, nil
	}
	ok, err := passwordpkg.VerifyPassword(ctx.FormValue("password"), user.Password)
	if err != nil {
		return false, ctx.ErrorRes(500, "Cannot verify password", err)
	}
	if !ok {
		ctx.AddFlash(ctx.Tr("flash.user.current-password-invalid"), "error")
		return false, ctx.RedirectTo("/settings/authentication")
	}
	return true, nil
}

func BeginTotp(ctx *context.Context) error {
	user := ctx.User

	if _, hasTotp, err := user.HasMFA(); err != nil {
		return ctx.ErrorRes(500, "Cannot check for user MFA", err)
	} else if hasTotp {
		ctx.AddFlash(ctx.Tr("auth.totp.already-enabled"), "error")
		return ctx.RedirectTo("/settings/authentication")
	}

	ogUrl, err := url.Parse(ctx.GetData("baseHttpUrl").(string))
	if err != nil {
		return ctx.ErrorRes(500, "Cannot parse base URL", err)
	}

	sess := ctx.GetSession()
	generatedSecret, _ := sess.Values["generatedSecret"].([]byte)

	totpSecret, qrcode, generatedSecret, err := totp.GenerateQRCode(ctx.User.Username, ogUrl.Hostname(), generatedSecret)
	if err != nil {
		return ctx.ErrorRes(500, "Cannot generate TOTP QR code", err)
	}
	sess.Values["totpSecret"] = totpSecret
	sess.Values["generatedSecret"] = generatedSecret
	ctx.SaveSession(sess)

	ctx.SetData("totpSecret", totpSecret)
	ctx.SetData("totpQrcode", qrcode)
	ctx.SetData("settingsHeaderPage", "authentication")

	return ctx.Html("totp.html")

}

func FinishTotp(ctx *context.Context) error {
	user := ctx.User

	if _, hasTotp, err := user.HasMFA(); err != nil {
		return ctx.ErrorRes(500, "Cannot check for user MFA", err)
	} else if hasTotp {
		ctx.AddFlash(ctx.Tr("auth.totp.already-enabled"), "error")
		return ctx.RedirectTo("/settings/authentication")
	}

	dto := &db.TOTPDTO{}
	if err := ctx.Bind(dto); err != nil {
		return ctx.ErrorRes(400, ctx.Tr("error.cannot-bind-data"), err)
	}

	if err := ctx.Validate(dto); err != nil {
		ctx.AddFlash(ctx.Tr("flash.auth.totp-invalid"), "error")
		return ctx.RedirectTo("/settings/totp/generate")
	}

	sess := ctx.GetSession()
	secret, ok := sess.Values["totpSecret"].(string)
	if !ok {
		return ctx.ErrorRes(500, "Cannot get TOTP secret from session", nil)
	}

	if !totp.Validate(dto.Code, secret) {
		ctx.AddFlash(ctx.Tr("auth.totp.invalid-code"), "error")

		return ctx.RedirectTo("/settings/totp/generate")
	}

	userTotp := &db.TOTP{
		UserID: ctx.User.ID,
	}
	if err := userTotp.StoreSecret(secret); err != nil {
		return ctx.ErrorRes(500, "Cannot store TOTP secret", err)
	}

	if err := userTotp.Create(); err != nil {
		return ctx.ErrorRes(500, "Cannot create TOTP", err)
	}

	ctx.AddFlash(ctx.Tr("flash.auth.totp-enabled"), "success")
	codes, err := userTotp.GenerateRecoveryCodes()
	if err != nil {
		return ctx.ErrorRes(500, "Cannot generate recovery codes", err)
	}

	delete(sess.Values, "totpSecret")
	delete(sess.Values, "generatedSecret")
	ctx.SaveSession(sess)

	ctx.SetData("recoveryCodes", codes)
	ctx.SetData("settingsHeaderPage", "authentication")
	return ctx.Html("totp.html")
}

func AssertTotp(ctx *context.Context) error {
	var err error
	dto := &db.TOTPDTO{}
	if err := ctx.Bind(dto); err != nil {
		return ctx.ErrorRes(400, ctx.Tr("error.cannot-bind-data"), err)
	}

	if err := ctx.Validate(dto); err != nil {
		ctx.AddFlash(ctx.Tr("auth.totp.invalid-code"), "error")
		return ctx.RedirectTo("/mfa")
	}

	sess := ctx.GetSession()
	userId := sess.Values["mfaID"].(uint)
	var userTotp *db.TOTP
	if userTotp, err = db.GetTOTPByUserID(userId); err != nil {
		return ctx.ErrorRes(500, "Cannot get TOTP by UID", err)
	}

	redirectUrl := "/"

	var validCode, validRecoveryCode bool
	if validCode, err = userTotp.ValidateCode(dto.Code); err != nil {
		return ctx.ErrorRes(500, "Cannot validate TOTP code", err)
	}
	if !validCode {
		validRecoveryCode, err = userTotp.ValidateRecoveryCode(dto.Code)
		if err != nil {
			return ctx.ErrorRes(500, "Cannot validate TOTP code", err)
		}

		if !validRecoveryCode {
			ctx.AddFlash(ctx.Tr("auth.totp.invalid-code"), "error")
			return ctx.RedirectTo("/mfa")
		}

		ctx.AddFlash(ctx.Tr("auth.totp.code-used", dto.Code), "warning")
		redirectUrl = "/settings/authentication"
	}

	var user *db.User
	if user, err = db.GetUserById(userId); err != nil {
		return ctx.ErrorRes(500, "Cannot get user", err)
	}

	sess.Values["user"] = userId
	sess.Values["sessionVersion"] = user.SessionVersion
	sess.Options.MaxAge = 60 * 60 * 24 * 365 // 1 year
	delete(sess.Values, "mfaID")
	ctx.SaveSession(sess)

	return ctx.RedirectTo(redirectUrl)
}

// UpdateTotp dispatches the two actions of the TOTP card. They share one form
// so both can be gated on the account password.
func UpdateTotp(ctx *context.Context) error {
	if done, errRes := confirmCurrentPassword(ctx, ctx.User); !done {
		return errRes
	}

	switch ctx.FormValue("action") {
	case "disable":
		return DisableTotp(ctx)
	case "regenerate":
		return RegenerateTotpRecoveryCodes(ctx)
	default:
		return ctx.ErrorRes(400, "Unknown action", nil)
	}
}

func DisableTotp(ctx *context.Context) error {
	user := ctx.User
	userTotp, err := db.GetTOTPByUserID(user.ID)
	if err != nil {
		return ctx.ErrorRes(500, "Cannot get TOTP by UID", err)
	}

	if err = userTotp.Delete(); err != nil {
		return ctx.ErrorRes(500, "Cannot delete TOTP", err)
	}

	ctx.AddFlash(ctx.Tr("auth.totp.disabled"), "success")
	return ctx.RedirectTo("/settings/authentication")
}

func RegenerateTotpRecoveryCodes(ctx *context.Context) error {
	user := ctx.User
	userTotp, err := db.GetTOTPByUserID(user.ID)
	if err != nil {
		return ctx.ErrorRes(500, "Cannot get TOTP by UID", err)
	}

	codes, err := userTotp.GenerateRecoveryCodes()
	if err != nil {
		return ctx.ErrorRes(500, "Cannot generate recovery codes", err)
	}

	ctx.SetData("recoveryCodes", codes)
	ctx.SetData("settingsHeaderPage", "authentication")
	return ctx.Html("totp.html")
}
