package gist

import (
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/web/context"
)

func DeleteGist(ctx *context.Context) error {
	gist := ctx.GetData("gist").(*db.Gist)

	if err := gist.Delete(); err != nil {
		return ctx.ErrorRes(500, "Error deleting this gist", err)
	}
	gist.RemoveFromIndex()

	ctx.AddFlash(ctx.Tr("flash.gist.deleted"), "success")
	return ctx.RedirectTo("/")
}
