package settings

import (
	"strconv"

	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/i18n"
	"github.com/qingyu-org/Gist/internal/validator"
	"github.com/qingyu-org/Gist/internal/web/context"
)

func AccessTokens(ctx *context.Context) error {
	user := ctx.User

	tokens, err := db.GetAccessTokensByUserID(user.ID)
	if err != nil {
		return ctx.ErrorRes(500, "Cannot get access tokens", err)
	}

	ctx.SetData("accessTokens", tokens)
	ctx.SetData("apiEnabled", config.C.ApiEnabled)
	ctx.SetData("userIsAdmin", user.IsAdmin)
	ctx.SetData("settingsHeaderPage", "tokens")
	ctx.SetData("htmlTitle", ctx.TrH("settings"))
	return ctx.Html("settings_tokens.html")
}

func AccessTokensProcess(ctx *context.Context) error {
	user := ctx.User

	// A token would be useless while the API is switched off, and the page
	// warns about that, but the form is still posted by anything that reaches
	// this route directly.
	if !config.C.ApiEnabled {
		ctx.AddFlash(ctx.Tr("settings.api-disabled-warning"), "error")
		return ctx.RedirectTo("/settings/access-tokens")
	}

	dto := new(db.AccessTokenDTO)
	if err := ctx.Bind(dto); err != nil {
		return ctx.ErrorRes(400, ctx.Tr("error.cannot-bind-data"), err)
	}

	if err := ctx.Validate(dto); err != nil {
		ctx.AddFlash(validator.ValidationMessages(&err, ctx.GetData("locale").(*i18n.Locale)), "error")
		return ctx.RedirectTo("/settings/access-tokens")
	}

	token := dto.ToAccessToken()
	token.UserID = user.ID

	plainToken, err := token.GenerateToken()
	if err != nil {
		return ctx.ErrorRes(500, "Cannot generate token", err)
	}

	if err := token.Create(); err != nil {
		return ctx.ErrorRes(500, "Cannot create access token", err)
	}

	// The plaintext token is only ever shown once, so it is delivered as a
	// "secret" flash: the page renders it in place and keeps it on screen until
	// the user dismisses it, unlike the auto-dismissing toasts.
	ctx.AddFlash(plainToken, "secret")
	return ctx.RedirectTo("/settings/access-tokens")
}

func AccessTokensDelete(ctx *context.Context) error {
	user := ctx.User
	tokenID, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return ctx.RedirectTo("/settings/access-tokens")
	}

	token, err := db.GetAccessTokenByID(uint(tokenID))
	if err != nil || token.UserID != user.ID {
		return ctx.RedirectTo("/settings/access-tokens")
	}

	if err := token.Delete(); err != nil {
		return ctx.ErrorRes(500, "Cannot delete access token", err)
	}

	ctx.AddFlash(ctx.Tr("settings.token-deleted"), "success")
	return ctx.RedirectTo("/settings/access-tokens")
}
