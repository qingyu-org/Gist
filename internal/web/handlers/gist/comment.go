package gist

import (
	"errors"
	"strconv"

	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/i18n"
	"github.com/qingyu-org/Gist/internal/render"
	"github.com/qingyu-org/Gist/internal/validator"
	"github.com/qingyu-org/Gist/internal/web/context"
	"github.com/qingyu-org/Gist/internal/web/handlers"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const (
	// lastCommentsCount is how many of the latest comments the gist page shows
	lastCommentsCount = 5
	// commentsPerPage is the page size of the comments page, which lists them all
	commentsPerPage = 30
)

// Comments lists every comment posted on the gist, oldest first, paginated.
// Pages hold whole threads: they are numbered by top-level comments, and the
// replies of a page's threads are loaded with them.
func Comments(ctx *context.Context) error {
	gist := ctx.GetData("gist").(*db.Gist)

	pageInt := handlers.GetPage(ctx)

	// One more than shown, to know if there is a next page
	roots, err := gist.GetRootCommentsPage(pageInt-1, commentsPerPage+1, commentsPerPage)
	if err != nil {
		return ctx.ErrorRes(500, "Error fetching comments", err)
	}

	if err = handlers.Paginate(ctx, roots, pageInt, commentsPerPage, "rootComments", gist.User.Username+"/"+gist.Identifier()+"/comments", 1, nil); err != nil {
		return ctx.ErrorRes(404, ctx.Tr("error.page-not-found"), nil)
	}

	// Paginate dropped the peek-next extra comment from the page
	roots = ctx.GetData("rootComments").([]*db.GistComment)
	if err = setCommentThreads(ctx, gist, roots); err != nil {
		return ctx.ErrorRes(500, "Error fetching comments", err)
	}

	ctx.SetData("page", "comments")
	ctx.SetData("htmlTitle", ctx.TrH("gist.comments.for", gist.Title))
	ctx.SetData("revision", "HEAD")
	// Comments can be answered from this page too
	ctx.SetData("canComment", !gist.Archived && (ctx.User != nil || config.C.CommentsAllowGuests))
	ctx.SetData("commentAsGuest", ctx.User == nil && config.C.CommentsAllowGuests)
	return ctx.Html("comments.html")
}

// CommentPermalink redirects to the comment, wherever it is shown.
func CommentPermalink(ctx *context.Context) error {
	gist := ctx.GetData("gist").(*db.Gist)

	comment, err := getComment(ctx, gist)
	if err != nil {
		return err
	}

	return redirectToComment(ctx, gist, comment)
}

// CreateComment posts a comment on the gist, either as the logged-in user or,
// when the instance allows it, as a guest with the name they typed in.
func CreateComment(ctx *context.Context) error {
	gist := ctx.GetData("gist").(*db.Gist)

	dto, ok := bindComment(ctx, gist)
	if !ok {
		return ctx.RedirectTo(gistUrl(gist) + "#comments")
	}

	comment := &db.GistComment{
		GistID:  gist.ID,
		Content: dto.Content,
	}
	if ctx.User != nil {
		comment.UserID = &ctx.User.ID
	} else {
		// The route lets guests through only when the instance allows it
		if dto.GuestName == "" {
			ctx.AddFlash(ctx.Tr("gist.comments.guest-name-required"), "error")
			return ctx.RedirectTo(gistUrl(gist) + "#comments")
		}
		comment.GuestName = dto.GuestName
	}

	if dto.ParentID != 0 {
		parent, err := db.GetGistComment(gist.ID, dto.ParentID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ctx.NotFound("Comment not found")
		} else if err != nil {
			return ctx.ErrorRes(500, "Error fetching comment", err)
		}
		// Replies are one level deep: answering a reply joins the same thread
		comment.ParentID = parent.ParentID
		if comment.ParentID == 0 {
			comment.ParentID = parent.ID
		}
	}

	if err := comment.Create(); err != nil {
		return ctx.ErrorRes(500, "Error creating comment", err)
	}
	// Preloaded on read, set here so the notification has the author's name
	comment.User = ctx.User

	if err := comment.NotifyParticipants(gist); err != nil {
		// The comment is posted either way: only the notification is lost
		log.Error().Err(err).Msgf("Comment: cannot notify about comment %d", comment.ID)
	}

	return redirectToComment(ctx, gist, comment)
}

// EditComment changes the content of a comment. Only its author can.
func EditComment(ctx *context.Context) error {
	gist := ctx.GetData("gist").(*db.Gist)

	comment, err := getComment(ctx, gist)
	if err != nil {
		return err
	}
	if !comment.CanEdit(ctx.User) {
		return ctx.ErrorRes(403, "You don't have permission to edit this comment", nil)
	}

	dto, ok := bindComment(ctx, gist)
	if !ok {
		return redirectToComment(ctx, gist, comment)
	}

	comment.Content = dto.Content
	if err = comment.Update(); err != nil {
		return ctx.ErrorRes(500, "Error updating comment", err)
	}

	return redirectToComment(ctx, gist, comment)
}

// DeleteComment removes a comment. Its author, the gist owner and admins can.
func DeleteComment(ctx *context.Context) error {
	gist := ctx.GetData("gist").(*db.Gist)

	comment, err := getComment(ctx, gist)
	if err != nil {
		return err
	}
	if !comment.CanDelete(ctx.User, gist) {
		return ctx.ErrorRes(403, "You don't have permission to delete this comment", nil)
	}

	// Located before deleting it, to go back to where it was shown
	page, err := commentPage(gist, comment)
	if err != nil {
		return ctx.ErrorRes(500, "Error locating comment", err)
	}

	if err = comment.Delete(); err != nil {
		return ctx.ErrorRes(500, "Error deleting comment", err)
	}

	ctx.AddFlash(ctx.Tr("flash.gist.comment-deleted"), "success")
	if page == 0 {
		return ctx.RedirectTo(gistUrl(gist) + "#comments")
	}
	return ctx.RedirectTo(commentsPageUrl(gist, page))
}

// bindComment binds and validates the comment form. On a validation error, it
// adds the error as a flash message and returns false.
func bindComment(ctx *context.Context, gist *db.Gist) (*db.GistCommentDTO, bool) {
	dto := new(db.GistCommentDTO)
	if err := ctx.Bind(dto); err != nil {
		ctx.AddFlash(ctx.Tr("error.cannot-bind-data"), "error")
		return nil, false
	}
	dto.Normalize()
	if err := ctx.Validate(dto); err != nil {
		ctx.AddFlash(validator.ValidationMessages(&err, ctx.GetData("locale").(*i18n.Locale)), "error")
		return nil, false
	}
	return dto, true
}

// getComment loads the comment from the :id route param. It is looked up
// within the gist so that a comment can't be reached through another gist.
func getComment(ctx *context.Context, gist *db.Gist) (*db.GistComment, error) {
	commentId, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		return nil, ctx.NotFound("Comment not found")
	}

	comment, err := db.GetGistComment(gist.ID, uint(commentId))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ctx.NotFound("Comment not found")
	} else if err != nil {
		return nil, ctx.ErrorRes(500, "Error fetching comment", err)
	}
	return comment, nil
}

// redirectToComment redirects to the comment on the gist page if it is one of
// the last comments shown there, or else on the page of the comments page
// listing it.
func redirectToComment(ctx *context.Context, gist *db.Gist, comment *db.GistComment) error {
	page, err := commentPage(gist, comment)
	if err != nil {
		return ctx.ErrorRes(500, "Error locating comment", err)
	}

	anchor := "#comment-" + strconv.FormatUint(uint64(comment.ID), 10)
	if page == 0 {
		return ctx.RedirectTo(gistUrl(gist) + anchor)
	}
	return ctx.RedirectTo(commentsPageUrl(gist, page) + anchor)
}

// commentPage returns the page of the comments page listing the comment, or 0
// if its thread is one of the last comments shown on the gist page. A reply
// is looked for through the top-level comment it belongs to, since pages hold
// whole threads.
func commentPage(gist *db.Gist, comment *db.GistComment) (int, error) {
	root := comment
	if !comment.IsRoot() {
		var err error
		root, err = db.GetGistComment(gist.ID, comment.ParentID)
		if err != nil {
			return 0, err
		}
	}

	position, err := root.RootPosition()
	if err != nil {
		return 0, err
	}
	total, err := gist.CountRootComments()
	if err != nil {
		return 0, err
	}
	if total-position <= lastCommentsCount {
		return 0, nil
	}
	return int(position)/commentsPerPage + 1, nil
}

// setCommentThreads loads the replies of the given top-level comments and sets
// the whole threads as the page's comments.
func setCommentThreads(ctx *context.Context, gist *db.Gist, roots []*db.GistComment) error {
	rootIDs := make([]uint, 0, len(roots))
	for _, root := range roots {
		rootIDs = append(rootIDs, root.ID)
	}

	replies, err := gist.GetReplies(rootIDs)
	if err != nil {
		return err
	}

	ctx.SetData("comments", render.RenderCommentThreads(append(roots, replies...)))
	return nil
}

func gistUrl(gist *db.Gist) string {
	return "/" + gist.User.Username + "/" + gist.Identifier()
}

func commentsPageUrl(gist *db.Gist, page int) string {
	return gistUrl(gist) + "/comments?page=" + strconv.Itoa(page)
}
