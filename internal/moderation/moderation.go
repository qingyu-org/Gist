// Package moderation runs the asynchronous AI compliance check that gates
// public gists. A gist submitted as public is stored privately with a pending
// status; a worker asks the OpenAI Responses API whether it may be published,
// then flips it to public or keeps it private with a stored reason.
//
// The check never runs on a request path: submissions enqueue an ID and return,
// so pushes and form posts are not held up by the provider's latency. Private
// and unlisted gists are not reviewed at all.
package moderation

import (
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/settings"
	"github.com/rs/zerolog/log"
)

const (
	queueSize = 128
	// workers lets one slow provider call coexist with another review instead
	// of serialising the queue behind it.
	workers = 2
	// retryInterval decides how often gists left pending by the daily quota are
	// picked up again.
	retryInterval = 10 * time.Minute
	// deferLogInterval limits the review log to one deferral row per gist per
	// hour while a backlog waits for the quota to reset.
	deferLogInterval = time.Hour
)

var (
	queueOnce sync.Once
	queue     chan uint

	// inflight deduplicates IDs so a retry tick that re-enqueues a gist already
	// waiting in the queue cannot produce two provider calls.
	flightMu sync.Mutex
	flight   = make(map[uint]struct{})
)

// Start launches the moderation workers. It is safe to call when moderation is
// disabled: the workers only act on queued gists and the setting is checked
// when a gist is enqueued.
func Start() {
	ensureWorkers()
	if !settings.ModerationEnabled() {
		return
	}
	// Resume checks that were interrupted by a restart.
	if ids, err := db.GetPendingModerationGistIDs(); err == nil {
		for _, id := range ids {
			Enqueue(id)
		}
	}
}

func ensureWorkers() {
	queueOnce.Do(func() {
		queue = make(chan uint, queueSize)
		for i := 0; i < workers; i++ {
			go worker()
		}
		go retryLoop()
	})
}

// Gate holds back a gist that was submitted as public until the worker has
// reviewed it. It returns true when the gist now needs to be enqueued; callers
// should still save the gist, then call Enqueue with its ID.
func Gate(gist *db.Gist) bool {
	if !settings.ModerationEnabled() || gist.Private != db.PublicVisibility || gist.ModerationStatus == db.ModerationApproved {
		return false
	}
	gist.Private = db.PrivateVisibility
	gist.ModerationStatus = db.ModerationPending
	return true
}

// Recheck marks an already-published gist for another review after its content
// changed. The gist stays online until the verdict arrives, so a normal edit
// never interrupts readers; only a rejection takes it down. Callers save the
// gist and then call Enqueue.
func Recheck(gist *db.Gist) bool {
	if !settings.ModerationEnabled() || gist.Private != db.PublicVisibility || gist.ModerationStatus != db.ModerationApproved {
		return false
	}
	gist.ModerationStatus = db.ModerationPending
	return true
}

// Release drops a pending check once the owner makes the gist non-public.
// Private and unlisted gists are never reviewed.
func Release(gist *db.Gist) {
	if gist.Private == db.PublicVisibility {
		return
	}
	if gist.ModerationStatus == db.ModerationPending {
		gist.ModerationStatus = db.ModerationNone
	}
}

// Enqueue schedules a gist for moderation without blocking the caller.
func Enqueue(gistID uint) {
	ensureWorkers()
	if !settings.ModerationEnabled() {
		return
	}
	if !claim(gistID) {
		return
	}
	select {
	case queue <- gistID:
	default:
		// The queue is saturated; run this one on its own goroutine rather than
		// leaving the gist pending forever.
		go reviewAsync(gistID)
	}
}

func claim(id uint) bool {
	flightMu.Lock()
	defer flightMu.Unlock()
	if _, busy := flight[id]; busy {
		return false
	}
	flight[id] = struct{}{}
	return true
}

func release(id uint) {
	flightMu.Lock()
	delete(flight, id)
	flightMu.Unlock()
}

func worker() {
	for id := range queue {
		reviewAsync(id)
	}
}

func reviewAsync(id uint) {
	defer release(id)
	moderate(id)
}

// retryLoop re-queues gists that were left pending, either by the daily quota
// or by a crash. Calls are cheap: the queue deduplicates and the quota check
// inside moderate decides whether the provider may be contacted again.
func retryLoop() {
	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()
	for range ticker.C {
		if !settings.ModerationEnabled() {
			continue
		}
		ids, err := db.GetPendingModerationGistIDs()
		if err != nil {
			log.Warn().Err(err).Msg("Moderation: cannot load pending gists")
			continue
		}
		for _, id := range ids {
			Enqueue(id)
		}
	}
}

func moderate(gistID uint) {
	gist, err := db.GetGistByID(strconv.FormatUint(uint64(gistID), 10))
	if err != nil {
		log.Error().Err(err).Msgf("Moderation: cannot load gist %d", gistID)
		return
	}
	if gist.ModerationStatus != db.ModerationPending {
		return
	}

	if ok, err := quotaAvailable(); err != nil {
		log.Warn().Err(err).Msg("Moderation: cannot read the daily quota")
	} else if !ok {
		deferGist(gist)
		return
	}

	document := document(gist)
	result, err := review(settings.ModerationPrompt(), document)
	if err != nil {
		log.Warn().Err(err).Msgf("Moderation: review failed for gist %d", gistID)
		latency, model := 0, ""
		if result != nil {
			latency, model = result.LatencyMs, result.Model
		}
		record(gist, "error", err.Error(), model, latency)
		if settings.ModerationFailClosed() {
			finish(gist, db.ModerationRejected, reasonError, false)
			return
		}
		finish(gist, db.ModerationApproved, "moderation unavailable, published by policy", true)
		return
	}

	if result.Allowed {
		record(gist, "approved", result.Reason, result.Model, result.LatencyMs)
		finish(gist, db.ModerationApproved, result.Reason, true)
		return
	}
	record(gist, "rejected", result.Reason, result.Model, result.LatencyMs)
	finish(gist, db.ModerationRejected, result.Reason, false)
}

// reasonError is stored on the gist when the provider could not be reached and
// the site is configured to fail closed. The review log keeps the detail; this
// is what the author sees, so it explains how to get the gist published.
const reasonError = "moderation service unavailable, please contact the administrator"

// quotaAvailable reports whether another provider call fits in today's budget.
// A limit of 0 means unlimited.
func quotaAvailable() (bool, error) {
	limit := settings.ModerationDailyLimit()
	if limit <= 0 {
		return true, nil
	}
	count, err := db.CountModerationReviewsSince(startOfDay(time.Now()))
	if err != nil {
		return true, err
	}
	return count < int64(limit), nil
}

func startOfDay(now time.Time) int64 {
	y, m, d := now.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location()).Unix()
}

// deferGist leaves the gist pending until the next retry tick and notes why.
func deferGist(gist *db.Gist) {
	since := time.Now().Add(-deferLogInterval).Unix()
	if n, err := db.CountModerationReviewsForGistSince(gist.ID, "deferred", since); err == nil && n > 0 {
		return
	}
	record(gist, "deferred", "daily moderation quota reached", "", 0)
	log.Info().Msgf("Moderation: gist %d deferred, daily quota reached", gist.ID)
}

// record keeps one row per attempt so administrators can audit what the
// provider was asked and answered.
func record(gist *db.Gist, result, reason, model string, latency int) {
	review := &db.ModerationReview{
		Result:      result,
		GistID:      gist.ID,
		UserID:      gist.UserID,
		Username:    gist.User.Username,
		GistURL:     gistLink(gist),
		GistTitle:   gist.Title,
		Reason:      reason,
		Model:       model,
		LatencyMs:   latency,
		ProviderID:  providerHost(),
		AttemptedAt: time.Now().Unix(),
	}
	if err := db.RecordModerationReview(review); err != nil {
		log.Error().Err(err).Msgf("Moderation: cannot record review for gist %d", gist.ID)
	}
}

func providerHost() string {
	raw := settings.ModerationEndpoint()
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

// gistLink is the path an administrator or author can open to see the gist.
// Gists created without an explicit URL are addressed by their uuid, which is
// what Identifier returns and what the web routes accept.
func gistLink(gist *db.Gist) string {
	user := gist.User.Username
	if user == "" {
		if u, err := db.GetUserById(gist.UserID); err == nil {
			user = u.Username
		}
	}
	identifier := gist.Identifier()
	if user == "" || identifier == "" {
		return ""
	}
	return "/" + url.PathEscape(user) + "/" + url.PathEscape(identifier)
}

func finish(gist *db.Gist, status db.ModerationStatus, reason string, publish bool) {
	visibility := db.PrivateVisibility
	if publish {
		visibility = db.PublicVisibility
	}
	if err := gist.SetModerationResult(status, reason, visibility, time.Now().Unix()); err != nil {
		log.Error().Err(err).Msgf("Moderation: cannot store result for gist %d", gist.ID)
		return
	}

	gist.ModerationStatus = status
	gist.ModerationReason = reason
	gist.Private = visibility
	// The visible state changed, so refresh the search document too.
	if publish {
		gist.AddInIndex()
	} else {
		gist.RemoveFromIndex()
	}

	if status == db.ModerationRejected {
		notifyRejected(gist, reason)
		log.Info().Msgf("Moderation: gist %d kept private: %s", gist.ID, reason)
		return
	}
	log.Info().Msgf("Moderation: gist %d approved", gist.ID)
}

// notifyRejected tells the author what happened and where to fix it. Deferred
// and transient errors are not notified: only a decision the author can act on.
func notifyRejected(gist *db.Gist, reason string) {
	link := gistLink(gist)
	if err := db.CreateNotification("moderation", gist.UserID,
		"notification.moderation.rejected.title",
		"notification.moderation.rejected.body",
		[]string{gist.Title, reason},
		link,
	); err != nil {
		log.Error().Err(err).Msgf("Moderation: cannot notify author of gist %d", gist.ID)
	}
}

func document(gist *db.Gist) string {
	budget := settings.ModerationMaxInputBytes()
	if budget <= 0 {
		budget = 16 * 1024
	}

	var b strings.Builder
	b.WriteString("Title: ")
	b.WriteString(gist.Title)
	b.WriteString("\nDescription: ")
	b.WriteString(gist.Description)
	b.WriteString("\n\n")

	total := 0
	if files, _, err := gist.Files("HEAD", true); err == nil {
		for _, file := range files {
			header := "\n----- " + file.Filename + " -----\n"
			if total+len(header) >= budget {
				break
			}
			b.WriteString(header)
			total += len(header)

			content := file.Content
			remaining := budget - total
			if remaining <= 0 {
				break
			}
			if len(content) > remaining {
				content = truncate(content, remaining)
			}
			b.WriteString(content)
			b.WriteString("\n")
			total += len(content)
		}
	}

	return b.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Do not cut a multi-byte character (Chinese, emoji) in half.
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max] + "…"
}
