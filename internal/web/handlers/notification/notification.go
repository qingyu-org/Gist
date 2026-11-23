// Package notification serves the per-user notification inbox: the list page
// and the two endpoints that clear the unread badge.
package notification

import (
	"html/template"
	"strconv"

	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/render"
	"github.com/qingyu-org/Gist/internal/web/context"
	"github.com/qingyu-org/Gist/internal/web/handlers"
)

const perPage = 30

// Item is one row ready for the template: the title and body are already
// resolved to the viewer's language, and broadcast bodies are already markdown.
type Item struct {
	ID        uint
	Kind      string
	Title     string
	Body      template.HTML
	Link      string
	CreatedAt int64
	Read      bool
}

// List renders the inbox, newest first.
func List(ctx *context.Context) error {
	pageInt := handlers.GetPage(ctx)
	ctx.SetData("htmlTitle", ctx.TrH("notifications.title"))

	rows, err := db.GetVisibleNotifications(ctx.User.ID, perPage+1, (pageInt-1)*perPage)
	if err != nil {
		return ctx.ErrorRes(500, "Cannot get notifications", err)
	}

	items := make([]*Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, toItem(ctx, row))
	}

	if err := handlers.Paginate(ctx, items, pageInt, perPage, "notifications", "notifications", 2, nil); err != nil {
		return ctx.ErrorRes(404, "Page not found", err)
	}

	unread, err := db.CountUnreadNotifications(ctx.User.ID)
	if err != nil {
		unread = 0
	}
	ctx.SetData("unreadCount", int(unread))

	return ctx.Html("notifications.html")
}

func toItem(ctx *context.Context, row *db.UserNotification) *Item {
	item := &Item{
		ID:        row.ID,
		Kind:      row.Kind,
		Title:     row.Title,
		Link:      row.Link,
		CreatedAt: row.CreatedAt,
		Read:      row.ReadAt > 0,
	}
	if row.Kind == "broadcast" {
		// Administrator-written text supports markdown, like the announcement.
		if html, err := render.MarkdownString(row.Body); err == nil {
			item.Body = template.HTML(html)
		} else {
			item.Body = template.HTML(template.HTMLEscapeString(row.Body))
		}
		return item
	}

	args := row.NotificationArgs()
	if row.TitleKey != "" {
		// Titles are plain sentences; the positional arguments belong to the
		// body, which names the gist and the reason.
		item.Title = ctx.Tr(row.TitleKey)
	}
	if row.BodyKey != "" {
		item.Body = template.HTML(template.HTMLEscapeString(ctx.Tr(row.BodyKey, args...)))
	}
	return item
}

// MarkRead clears the flag on one notification and returns to the inbox
// (or to the page the bell was clicked from).
func MarkRead(ctx *context.Context) error {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err := db.MarkNotificationRead(ctx.User.ID, uint(id)); err != nil {
		return ctx.ErrorRes(500, "Cannot mark this notification as read", err)
	}
	return ctx.RedirectTo("/notifications")
}

// MarkAllRead clears the whole inbox in one click.
func MarkAllRead(ctx *context.Context) error {
	if err := db.MarkAllNotificationsRead(ctx.User.ID); err != nil {
		return ctx.ErrorRes(500, "Cannot mark notifications as read", err)
	}
	ctx.AddFlash(ctx.Tr("flash.notifications.all-read"), "success")
	return ctx.RedirectTo("/notifications")
}
