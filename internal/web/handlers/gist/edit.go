package gist

import (
	"strconv"

	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/moderation"
	"github.com/qingyu-org/Gist/internal/render"
	"github.com/qingyu-org/Gist/internal/web/context"
)

func Edit(ctx *context.Context) error {
	gist := ctx.GetData("gist").(*db.Gist)

	gistDto, err := gist.ToDTO()
	if err != nil {
		return ctx.ErrorRes(500, "Error getting gist data", err)
	}

	ctx.SetData("dto", gistDto)
	ctx.SetData("htmlTitle", ctx.TrH("gist.edit.edit-gist", gist.Title))

	return ctx.Html("edit.html")
}

func Checkbox(ctx *context.Context) error {
	filename := ctx.FormValue("file")
	checkboxNb := ctx.FormValue("checkbox")

	i, err := strconv.Atoi(checkboxNb)
	if err != nil {
		return ctx.ErrorRes(400, ctx.Tr("error.invalid-number"), nil)
	}

	gist := ctx.GetData("gist").(*db.Gist)
	file, err := gist.File("HEAD", filename, false)
	if err != nil {
		return ctx.ErrorRes(500, "Error getting file content", err)
	} else if file == nil {
		return ctx.NotFound("File not found")
	}

	markdown, err := render.Checkbox(file.Content, i)
	if err != nil {
		return ctx.ErrorRes(500, "Error checking checkbox", err)
	}

	if err = gist.AddAndCommitFile(&db.FileDTO{
		Filename: filename,
		Content:  markdown,
	}); err != nil {
		return ctx.ErrorRes(500, "Error adding and committing files", err)
	}

	if err = gist.UpdatePreviewAndCount(true); err != nil {
		return ctx.ErrorRes(500, "Error updating the gist", err)
	}

	return ctx.PlainText(200, "ok")
}

func ToggleArchive(ctx *context.Context) error {
	gist := ctx.GetData("gist").(*db.Gist)

	if err := gist.SetArchived(!gist.Archived); err != nil {
		return ctx.ErrorRes(500, "Error updating this gist", err)
	}

	if gist.Archived {
		ctx.AddFlash(ctx.Tr("flash.gist.archived"), "success")
	} else {
		ctx.AddFlash(ctx.Tr("flash.gist.unarchived"), "success")
	}
	return ctx.RedirectTo("/" + gist.User.Username + "/" + gist.Identifier())
}

func EditVisibility(ctx *context.Context) error {
	gist := ctx.GetData("gist").(*db.Gist)

	dto := new(db.VisibilityDTO)
	if err := ctx.Bind(dto); err != nil {
		return ctx.ErrorRes(400, ctx.Tr("error.cannot-bind-data"), err)
	}

	gist.Private = dto.Private

	// Publishing always goes through moderation when it is enabled, including
	// visibility changes made after the gist was created. Going the other way
	// drops any pending check: private and unlisted gists are not reviewed.
	awaitingModeration := false
	if dto.Private == db.PublicVisibility {
		awaitingModeration = moderation.Gate(gist)
	} else {
		moderation.Release(gist)
	}

	if err := gist.UpdateNoTimestamps(); err != nil {
		return ctx.ErrorRes(500, "Error updating this gist", err)
	}

	gist.AddInIndex()

	if awaitingModeration {
		moderation.Enqueue(gist.ID)
		ctx.AddFlash(ctx.Tr("flash.gist.moderation-pending"), "info")
	} else {
		ctx.AddFlash(ctx.Tr("flash.gist.visibility-changed"), "success")
	}
	return ctx.RedirectTo("/" + gist.User.Username + "/" + gist.Identifier())
}
