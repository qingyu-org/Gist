package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/moderation"
	"github.com/qingyu-org/Gist/internal/settings"
	"github.com/qingyu-org/Gist/internal/web/context"
	"github.com/qingyu-org/Gist/internal/web/handlers"
)

const adminPageSize = 30

// AdminModeration lists every moderation attempt: what the provider was asked,
// what it answered and how long it took. The gist itself only keeps the latest
// verdict, so this log is the audit trail.
func AdminModeration(ctx *context.Context) error {
	ctx.SetData("htmlTitle", ctx.TrH("admin.moderation.title")+" - "+ctx.TrH("admin.admin_panel"))
	ctx.SetData("adminHeaderPage", "moderation")

	pageInt := handlers.GetPage(ctx)
	reviews, err := db.GetModerationReviews(adminPageSize+1, (pageInt-1)*adminPageSize)
	if err != nil {
		return ctx.ErrorRes(500, "Cannot get moderation reviews", err)
	}

	if err := handlers.Paginate(ctx, reviews, pageInt, adminPageSize, "reviews", "admin/moderation", 2, nil); err != nil {
		return ctx.ErrorRes(404, "Page not found", err)
	}

	total, err := db.CountModerationReviews()
	if err != nil {
		total = 0
	}
	stats, err := db.GetModerationReviewStats(time.Now().Add(-7 * 24 * time.Hour).Unix())
	if err != nil {
		stats = map[string]int64{}
	}
	pending, err := db.GetPendingModerationGistIDs()
	if err != nil {
		pending = nil
	}

	ctx.SetData("reviewTotal", total)
	ctx.SetData("stats", stats)
	ctx.SetData("pendingCount", len(pending))
	ctx.SetData("moderationEnabled", settings.ModerationEnabled())
	return ctx.Html("admin_moderation.html")
}

// AdminModerationRetry re-queues every gist that is still pending, which is how
// a backlog left by the daily quota is drained without waiting for the next
// automatic retry.
func AdminModerationRetry(ctx *context.Context) error {
	ids, err := db.GetPendingModerationGistIDs()
	if err != nil {
		return ctx.ErrorRes(500, "Cannot get pending gists", err)
	}
	for _, id := range ids {
		moderation.Enqueue(id)
	}
	ctx.AddFlash(ctx.Tr("flash.admin.moderation-requeued", len(ids)), "success")
	return ctx.RedirectTo("/admin/moderation")
}

// AdminNotifications is where site-wide notices are written: the list of what
// has been sent plus the composer for a new one.
func AdminNotifications(ctx *context.Context) error {
	ctx.SetData("htmlTitle", ctx.TrH("admin.notifications.title")+" - "+ctx.TrH("admin.admin_panel"))
	ctx.SetData("adminHeaderPage", "notifications")

	pageInt := handlers.GetPage(ctx)

	const perPage = 10
	rows, err := db.GetBroadcastNotifications(perPage+1, (pageInt-1)*perPage)
	if err != nil {
		return ctx.ErrorRes(500, "Cannot get notifications", err)
	}

	total, err := db.CountBroadcastNotifications()
	if err != nil {
		return ctx.ErrorRes(500, "Cannot get notifications", err)
	}

	// Paginate trims the extra row that signals a next page.
	if err = handlers.Paginate(ctx, rows, pageInt, perPage, "broadcasts", "admin/notifications", 2, nil); err != nil {
		return ctx.ErrorRes(404, ctx.Tr("error.page-not-found"), nil)
	}
	ctx.SetData("broadcastTotal", int(total))
	return ctx.Html("admin_notifications.html")
}

// AdminNotificationsCreate publishes a notice to every account, including the
// accounts created later: broadcasts are stored once with user_id 0.
func AdminNotificationsCreate(ctx *context.Context) error {
	title := strings.TrimSpace(ctx.FormValue("title"))
	body := strings.TrimSpace(ctx.FormValue("body"))
	link := strings.TrimSpace(ctx.FormValue("link"))

	if title == "" || body == "" {
		ctx.AddFlash(ctx.Tr("flash.admin.notification-empty"), "error")
		return ctx.RedirectTo("/admin/notifications")
	}
	if len([]rune(title)) > 200 || len([]rune(body)) > 10000 {
		ctx.AddFlash(ctx.Tr("flash.admin.notification-too-long"), "error")
		return ctx.RedirectTo("/admin/notifications")
	}
	// Only site-relative links or absolute http(s) URLs are accepted, so a
	// notice cannot inject a javascript: target.
	if link != "" &&
		!strings.HasPrefix(link, "/") &&
		!strings.HasPrefix(link, "https://") &&
		!strings.HasPrefix(link, "http://") {
		ctx.AddFlash(ctx.Tr("flash.admin.notification-bad-link"), "error")
		return ctx.RedirectTo("/admin/notifications")
	}

	if _, err := db.CreateBroadcast(title, body, link); err != nil {
		return ctx.ErrorRes(500, "Cannot create notification", err)
	}
	ctx.AddFlash(ctx.Tr("flash.admin.notification-created"), "success")
	return ctx.RedirectTo("/admin/notifications")
}

// AdminNotificationsDelete removes a broadcast for everybody.
func AdminNotificationsDelete(ctx *context.Context) error {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 64)
	if err := db.DeleteNotification(uint(id)); err != nil {
		return ctx.ErrorRes(500, "Cannot delete notification", err)
	}
	ctx.AddFlash(ctx.Tr("flash.admin.notification-deleted"), "success")
	return ctx.RedirectTo("/admin/notifications")
}
