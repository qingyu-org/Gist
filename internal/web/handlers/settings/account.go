package settings

import (
	"crypto/md5"
	"fmt"
	"strings"

	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/git"
	"github.com/qingyu-org/Gist/internal/i18n"
	qingyugistssh "github.com/qingyu-org/Gist/internal/ssh"
	"github.com/qingyu-org/Gist/internal/validator"
	"github.com/qingyu-org/Gist/internal/web/context"
)

func EmailProcess(ctx *context.Context) error {
	user := ctx.User
	email := strings.ToLower(strings.TrimSpace(ctx.FormValue("email")))

	if email == "" {
		// No address, no picture: the account falls back to the default avatar
		// (or a generated identicon) until an address is set.
		user.MD5Hash = ""
	} else {
		user.MD5Hash = fmt.Sprintf("%x", md5.Sum([]byte(email)))
	}

	user.Email = email

	if err := user.Update(); err != nil {
		return ctx.ErrorRes(500, "Cannot update email", err)
	}

	ctx.AddFlash(ctx.Tr("flash.user.email-updated"), "success")
	return ctx.RedirectTo("/settings")
}

func AccountDeleteProcess(ctx *context.Context) error {
	user := ctx.User

	if err := user.Delete(); err != nil {
		return ctx.ErrorRes(500, "Cannot delete this user", err)
	}
	qingyugistssh.SyncAuthorizedKeysLogged()

	// The flash lives in its own cookie, so it still reaches the home page after
	// the session itself is dropped.
	ctx.AddFlash(ctx.Tr("flash.settings.account-deleted"), "success")
	ctx.DeleteSession()

	return ctx.RedirectTo("/explore")
}

func UsernameProcess(ctx *context.Context) error {
	user := ctx.User

	dto := new(db.UserUsernameDTO)
	if err := ctx.Bind(dto); err != nil {
		return ctx.ErrorRes(400, ctx.Tr("error.cannot-bind-data"), err)
	}

	if err := ctx.Validate(dto); err != nil {
		ctx.AddFlash(validator.ValidationMessages(&err, ctx.GetData("locale").(*i18n.Locale)), "error")
		return ctx.RedirectTo("/settings")
	}

	if !strings.EqualFold(dto.Username, user.Username) {
		if exists, err := db.UserExists(dto.Username); err != nil || exists {
			ctx.AddFlash(ctx.Tr("flash.auth.username-exists"), "error")
			return ctx.RedirectTo("/settings")
		}
	}

	if err := git.RenameUserDirectory(user.Username, dto.Username); err != nil {
		return ctx.ErrorRes(500, "Cannot rename user directory", err)
	}

	user.Username = dto.Username

	if err := user.Update(); err != nil {
		return ctx.ErrorRes(500, "Cannot update username", err)
	}

	ctx.AddFlash(ctx.Tr("flash.user.username-updated"), "success")
	return ctx.RedirectTo("/settings")
}
