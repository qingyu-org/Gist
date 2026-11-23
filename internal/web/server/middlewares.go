package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/qingyu-org/Gist/internal/auth"
	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/i18n"
	"github.com/qingyu-org/Gist/internal/ipc"
	"github.com/qingyu-org/Gist/internal/render"
	"github.com/qingyu-org/Gist/internal/settings"
	"github.com/qingyu-org/Gist/internal/web/context"
	"github.com/qingyu-org/Gist/internal/web/handlers"
	"github.com/rs/zerolog/log"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gorm.io/gorm"
)

func (s *Server) useCustomContext() {
	s.echo.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			cc := context.NewContext(c, filepath.Join(config.GetHomeDir(), "sessions"))
			return next(cc)
		}
	})
}

func (s *Server) registerMiddlewares() {
	s.echo.Use(Middleware(dataInit).toEcho())
	s.echo.Use(Middleware(locale).toEcho())
	if config.C.MetricsEnabled {
		s.echo.Use(echoprometheus.NewMiddleware("qingyugist"))
	}

	s.echo.Pre(middleware.MethodOverrideWithConfig(middleware.MethodOverrideConfig{
		Getter: methodFromForm,
	}))
	s.echo.Pre(middleware.RemoveTrailingSlash())
	// Expose the API's pagination headers so cross-origin browser clients can
	// read them (the CORS spec hides non-safelisted response headers by default).
	s.echo.Pre(dynamicCORS)
	s.echo.Pre(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogURI: true, LogStatus: true, LogMethod: true,
		LogValuesFunc: func(ctx echo.Context, v middleware.RequestLoggerValues) error {
			log.Info().Str("uri", v.URI).Int("status", v.Status).Str("method", v.Method).
				Str("ip", ctx.RealIP()).TimeDiff("duration", time.Now(), v.StartTime).
				Msg("HTTP")
			return nil
		},
	}))
	s.echo.Use(middleware.Recover())
	s.echo.Use(middleware.Secure())
	s.echo.Use(Middleware(sessionInit).toEcho())
	s.echo.Use(Middleware(gistFormSize).toEcho())
	s.echo.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup:    "form:_csrf,header:X-CSRF-Token",
		CookiePath:     "/",
		CookieHTTPOnly: true,
		CookieSameSite: http.SameSiteStrictMode,
		Skipper: func(ctx echo.Context) bool {
			// skip CSRF for /api (uses bearer tokens, not session cookies); this
			// also covers the token-authenticated IPC API under /api/ipc
			if strings.HasPrefix(ctx.Request().URL.Path, "/api/") {
				return true
			}
			/* skip CSRF for embeds */
			gistName := ctx.Param("gistname")
			/* skip CSRF for git clients */
			matchUploadPack, _ := regexp.MatchString("(.*?)/git-upload-pack$", ctx.Request().URL.Path)
			matchReceivePack, _ := regexp.MatchString("(.*?)/git-receive-pack$", ctx.Request().URL.Path)
			return (filepath.Ext(gistName) == ".js" && ctx.Request().Method == "GET") || matchUploadPack || matchReceivePack
		},
		ErrorHandler: func(err error, c echo.Context) error {
			log.Info().Err(err).Msg("CSRF error")
			return err
		},
	}))
	s.echo.Use(Middleware(csrfInit).toEcho())

}

func (s *Server) errorHandler(err error, ctx echo.Context) {
	data, _ := ctx.Request().Context().Value(context.DataKeyStr).(echo.Map)
	if data == nil {
		data = echo.Map{}
	}

	var httpErr *context.HTTPError
	if errors.As(err, &httpErr) {
		data["error"] = err
	} else {
		if isClientGone(err) {
			return
		}
		log.Error().Err(err).Send()
		httpErr = &context.HTTPError{Message: err.Error(), Code: http.StatusInternalServerError}
		data["error"] = httpErr
	}

	if ctx.Response().Committed {
		return
	}

	acceptJson := strings.Contains(ctx.Request().Header.Get("Accept"), "application/json")
	var renderErr error
	if acceptJson || data["err_render"] == "json" {
		renderErr = ctx.JSON(httpErr.Code, httpErr)
	} else {
		renderErr = ctx.Render(httpErr.Code, "error.html", data)
	}

	if renderErr != nil && !isClientGone(renderErr) {
		log.Error().Err(renderErr).Send()
	}
}

func isClientGone(err error) bool {
	return errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)
}

func dataInit(next Handler) Handler {
	return func(ctx *context.Context) error {
		ctx.SetData("loadStartTime", time.Now())
		ctx.SetData("cspNonce", newCSPNonce())

		// set before loadSettings can fail: the error page renders the same
		// layout, which reads .c, so without it the error page itself fails
		ctx.SetData("c", config.C)

		if err := loadSettings(ctx); err != nil {
			return ctx.ErrorRes(500, "Cannot load settings", err)
		}

		ctx.SetData("githubOauth", config.C.GithubOAuthEnabled())
		ctx.SetData("googleOauth", config.C.GoogleOAuthEnabled())
		ctx.SetData("gitlabOauth", config.C.GitlabOAuthEnabled())
		ctx.SetData("giteaOauth", config.C.GiteaOAuthEnabled())
		ctx.SetData("oidcOauth", config.C.OpenIDConnectEnabled())
		ctx.SetData("oauth2Oauth", config.C.OAuth2OAuthEnabled())

		httpProtocol := "http"
		if ctx.Request().TLS != nil || ctx.Request().Header.Get("X-Forwarded-Proto") == "https" {
			httpProtocol = "https"
		}
		ctx.SetData("httpProtocol", strings.ToUpper(httpProtocol))

		var baseHttpUrl string
		// if a custom external url is set, use it
		if config.C.ExternalUrl != "" {
			baseHttpUrl = config.C.ExternalUrl
		} else {
			baseHttpUrl = httpProtocol + "://" + ctx.Request().Host
		}

		ctx.SetData("baseHttpUrl", baseHttpUrl)
		ctx.SetData("canonicalUrl", baseHttpUrl+ctx.Request().URL.Path)

		return next(ctx)
	}
}

func writePermission(next Handler) Handler {
	return func(ctx *context.Context) error {
		gist := ctx.GetData("gist").(*db.Gist)
		user := ctx.User
		if !gist.CanWrite(user) {
			return ctx.ErrorRes(403, "You don't have permission to edit this gist", nil)
		}
		return next(ctx)
	}
}

// notArchived blocks write operations on an archived (read-only) gist. It must
// run after gistInit so the gist is available in the context.
func notArchived(next Handler) Handler {
	return func(ctx *context.Context) error {
		gist := ctx.GetData("gist").(*db.Gist)
		if gist.Archived {
			return ctx.ErrorRes(403, "This gist is archived and is read-only", nil)
		}
		return next(ctx)
	}
}

// commentsEnabled blocks the gist comment routes when comments are disabled in
// the config.
func commentsEnabled(next Handler) Handler {
	return func(ctx *context.Context) error {
		if !config.C.CommentsEnabled {
			return ctx.ErrorRes(403, ctx.Tr("error.comments-disabled"), nil)
		}
		return next(ctx)
	}
}

func adminPermission(next Handler) Handler {
	return func(ctx *context.Context) error {
		user := ctx.User
		if user == nil || !user.IsAdmin {
			return ctx.NotFound("User not found")
		}
		return next(ctx)
	}
}

func logged(next Handler) Handler {
	return func(ctx *context.Context) error {
		user := ctx.User
		if user != nil {
			return next(ctx)
		}
		return ctx.RedirectTo("/explore")
	}
}

// commentable lets signed-in users post comments, and guests too when the
// instance allows anonymous comments. Anyone else is sent to the explore page,
// like the logged middleware does.
func commentable(next Handler) Handler {
	return func(ctx *context.Context) error {
		if ctx.User != nil || config.C.CommentsAllowGuests {
			return next(ctx)
		}
		return ctx.RedirectTo("/explore")
	}
}

func inMFASession(next Handler) Handler {
	return func(ctx *context.Context) error {
		sess := ctx.GetSession()
		_, ok := sess.Values["mfaID"].(uint)
		if !ok {
			return ctx.ErrorRes(400, ctx.Tr("error.not-in-mfa-session"), nil)
		}
		return next(ctx)
	}
}

func inOAuthRegisterSession(next Handler) Handler {
	return func(ctx *context.Context) error {
		sess := ctx.GetSession()
		_, ok := sess.Values["oauthProvider"].(string)
		if !ok {
			return ctx.RedirectTo("/login")
		}
		return next(ctx)
	}
}

func makeCheckRequireLogin(isSingleGistAccess bool) Middleware {
	return func(next Handler) Handler {
		return func(ctx *context.Context) error {
			if user := ctx.User; user != nil {
				return next(ctx)
			}

			if getUserByToken(ctx) != nil {
				return next(ctx)
			}
			allow, err := auth.ShouldAllowUnauthenticatedGistAccess(handlers.ContextAuthInfo{Context: ctx}, isSingleGistAccess)
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to check if unauthenticated access is allowed")
			}

			if !allow {
				ctx.AddFlash(ctx.Tr("flash.auth.must-be-logged-in"), "error")
				return ctx.RedirectTo("/login")
			}
			return next(ctx)
		}
	}
}

func checkRequireLogin(next Handler) Handler {
	return makeCheckRequireLogin(false)(next)
}

func checkFileUploadEnabled(next Handler) Handler {
	return func(ctx *context.Context) error {
		if config.C.DisableFileUpload {
			return ctx.ErrorRes(403, ctx.Tr("error.file-upload-disabled"), nil)
		}
		return next(ctx)
	}
}

// makeApiCheckRequireLogin is the /api/v1 counterpart of makeCheckRequireLogin:
// it enforces the instance's RequireLogin / AllowGistsWithoutLogin settings on
// anonymous gist reads, but responds with a JSON 401 instead of redirecting to
// /login. ctx.User is already resolved from the Authorization header by
// apiBindAuth, so there is no token fallback to do here.
func makeApiCheckRequireLogin(isSingleGistAccess bool) Middleware {
	return func(next Handler) Handler {
		return func(ctx *context.Context) error {
			if ctx.User != nil {
				return next(ctx)
			}

			allow, err := auth.ShouldAllowUnauthenticatedGistAccess(handlers.ContextAuthInfo{Context: ctx}, isSingleGistAccess)
			if err != nil {
				return ctx.ErrorJson(500, "Failed to check if unauthenticated access is allowed", err)
			}

			if !allow {
				return ctx.ErrorJson(401, "Requires authentication", nil)
			}
			return next(ctx)
		}
	}
}

func noRouteFound(ctx *context.Context) error {
	return ctx.NotFound("Page not found")
}

func noRouteFoundApi(ctx *context.Context) error {
	return ctx.ErrorJson(404, "Not found", nil)
}

func locale(next Handler) Handler {
	return func(ctx *context.Context) error {
		// Check URL arguments
		lang := ctx.Request().URL.Query().Get("lang")
		changeLang := lang != ""

		// Then check cookies
		if len(lang) == 0 {
			cookie, _ := ctx.Request().Cookie("lang")
			if cookie != nil {
				lang = cookie.Value
			}
		}

		// Check again in case someone changes the supported language list.
		if lang != "" && !i18n.Locales.Usable(lang) {
			lang = ""
			changeLang = false
		}

		// 3.Then check from 'Accept-Language' header.
		if len(lang) == 0 {
			lang = i18n.Locales.MatchTag(i18n.ParseAcceptLanguage(ctx.Request().Header.Get("Accept-Language")))
		}

		if changeLang {
			ctx.SetCookie(&http.Cookie{Name: "lang", Value: lang, Path: "/", MaxAge: 1<<31 - 1})
		}

		localeUsed, err := i18n.Locales.GetLocale(lang)
		if err != nil {
			return ctx.ErrorRes(500, "Cannot get locale", err)
		}

		ctx.SetData("localeName", localeUsed.Name)
		ctx.SetData("locale", localeUsed)
		ctx.SetData("requestUri", ctx.Request().URL.RequestURI())
		ctx.SetData("allLocales", i18n.Locales.Offered())

		return next(ctx)
	}
}

func sessionInit(next Handler) Handler {
	return func(ctx *context.Context) error {
		sess := ctx.GetSession()
		if sess.Values["user"] != nil {
			var err error
			var user *db.User

			// log out the session if its user no longer exists, or if it was opened
			// before the user's credentials changed (older sessions without a
			// version count as version 0)
			sessionVersion, _ := sess.Values["sessionVersion"].(uint)
			if user, err = db.GetUserById(sess.Values["user"].(uint)); err != nil || user.SessionVersion != sessionVersion {
				sess.Values["user"] = nil
				ctx.SaveSession(sess)
				ctx.User = nil
				ctx.SetData("userLogged", nil)
				return ctx.RedirectTo("/explore")
			}
			if user != nil {
				ctx.User = user
				ctx.SetData("userLogged", user)
				ctx.SetData("currentStyle", user.GetStyle())
				// The header bell shows the badge on every page, so the count
				// travels with the session rather than with the inbox handler.
				if unread, err := db.CountUnreadNotifications(user.ID); err == nil {
					ctx.SetData("UnreadNotifications", int(unread))
				}
			}
			return next(ctx)
		}

		ctx.User = nil
		ctx.SetData("userLogged", nil)
		return next(ctx)
	}
}

// newCSPNonce returns a fresh random nonce for the Content-Security-Policy of
// the current request. The same value is exposed to templates (so inline
// scripts can carry a matching nonce attribute) and used in the CSP header set
// by handlers that opt into a strict policy (e.g. the gist view page).
func newCSPNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// rand.Read never returns an error on supported platforms; fall back to
		// an empty nonce rather than serving a non-functional inline script.
		return ""
	}
	return base64.RawStdEncoding.EncodeToString(b)
}

func csrfInit(next Handler) Handler {
	return func(ctx *context.Context) error {
		var csrf string
		if csrfToken, ok := ctx.Get("csrf").(string); ok {
			csrf = csrfToken
		}
		ctx.SetData("csrfHtml", template.HTML(`<input type="hidden" name="_csrf" value="`+csrf+`">`))

		return next(ctx)
	}
}

func loadSettings(ctx *context.Context) error {
	// Pick up writes made through the db package (legacy route, tests) without
	// querying on every request.
	if err := settings.EnsureFresh(); err != nil {
		return err
	}
	ctx.SetData("c", config.C)

	snap := settings.Snapshot()
	legacy := func(key string) bool { return snap[key] == "1" }

	for key, value := range snap {
		d, ok := settings.Define(key)
		if !ok || d.Kind != settings.KindBool {
			continue
		}
		s := strings.ReplaceAll(key, "-", " ")
		s = strings.ReplaceAll(s, ".", " ")
		s = cases.Title(language.English).String(s)
		ctx.SetData(strings.ReplaceAll(s, " ", ""), value == "1")
	}

	// Explicit names used by templates. Keeping these stable avoids leaking
	// the registry's key format into every page.
	ctx.SetData("RegistrationMode", settings.RegistrationMode())
	ctx.SetData("OAuthOnlyRegistration", settings.OAuthOnlyRegistration())
	// A legacy key written directly into the settings table (older releases,
	// tests, third-party tooling) still has to switch the feature off.
	ctx.SetData("DisableSignup", settings.ClosedRegistration() || legacy(db.SettingDisableSignup))
	ctx.SetData("RequireLogin", settings.RequireLogin() || legacy(db.SettingRequireLogin))
	ctx.SetData("AllowGistsWithoutLogin", settings.AllowGistsWithoutLogin() || legacy(db.SettingAllowGistsWithoutLogin))
	ctx.SetData("DisableLoginForm", settings.DisableLoginForm() || legacy(db.SettingDisableLoginForm))
	// Avatars: the templates only need to know whether a picture exists for a
	// given user (see the avatarUrl helper); these two describe the provider
	// itself for the account settings page.
	ctx.SetData("AvatarProviderName", settings.AvatarProviderDisplayName())
	ctx.SetData("AvatarProviderSite", settings.AvatarProviderSite())
	ctx.SetData("DisableFileUpload", !settings.UploadEnabled())
	ctx.SetData("UploadEnabled", settings.UploadEnabled())
	ctx.SetData("SiteName", settings.SiteName())
	ctx.SetData("SiteDescription", settings.SiteDescription())
	ctx.SetData("CustomCSS", template.CSS(settings.CustomCSS()))
	ctx.SetData("CustomJS", template.JS(settings.CustomJS()))
	ctx.SetData("SiteRepositoryURL", settings.SiteRepositoryURL())
	ctx.SetData("SiteHomepageURL", settings.SiteHomepageURL())
	ctx.SetData("SiteContactEmail", settings.SiteContactEmail())
	ctx.SetData("SiteICP", settings.SiteICP())
	ctx.SetData("SitePoliceRecord", settings.SitePoliceRecord())
	ctx.SetData("SiteLinks", settings.SiteLinks())
	ctx.SetData("SiteLinkIcons", settings.SiteLinkIcons())
	ctx.SetData("SiteAnnouncement", announcementHTML(settings.SiteAnnouncement()))
	ctx.SetData("ModerationEnabled", settings.ModerationEnabled())
	ctx.SetData("settings", settings.Snapshot())
	return nil
}

// renderedAnnouncement memoises the announcement markdown: the value is the
// same for every request until an administrator edits it, and rendering it on
// each page load would be wasted work.
var renderedAnnouncement struct {
	sync.Mutex
	raw  string
	html template.HTML
}

// announcementHTML turns the administrator's markdown into the banner markup.
// An empty setting hides the banner; a document the renderer rejects falls back
// to escaped text instead of failing the page.
func announcementHTML(raw string) template.HTML {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	renderedAnnouncement.Lock()
	defer renderedAnnouncement.Unlock()
	if renderedAnnouncement.raw == raw {
		return renderedAnnouncement.html
	}

	out, err := render.MarkdownString(raw)
	if err != nil {
		out = template.HTMLEscapeString(raw)
	}
	renderedAnnouncement.raw = raw
	renderedAnnouncement.html = template.HTML(out)
	return renderedAnnouncement.html
}

func apiScope(scope, permission uint) Middleware {
	return func(next Handler) Handler {
		return func(ctx *context.Context) error {
			tok, ok := ctx.GetData("accessToken").(*db.AccessToken)
			if !ok || !tok.CheckForPermission(scope, permission) {
				return ctx.ErrorJson(403, "token lacks required scope/permission", nil)
			}
			return next(ctx)
		}
	}
}

// getUserByToken checks the Authorization header for token-based auth.
// Expects format: Authorization: Token <token>
// Returns the user if the token is valid and has gist read permission, nil otherwise.
func getUserByToken(ctx *context.Context) *db.User {
	authHeader := ctx.Request().Header.Get("Authorization")
	if authHeader == "" {
		return nil
	}

	if !strings.HasPrefix(authHeader, "Token ") {
		return nil
	}

	plainToken := strings.TrimPrefix(authHeader, "Token ")

	accessToken, err := db.GetAccessTokenByToken(plainToken)
	if err != nil {
		return nil
	}

	if accessToken.IsExpired() {
		return nil
	}

	if !accessToken.HasGistReadPermission() {
		return nil
	}

	_ = accessToken.UpdateLastUsed()

	return &accessToken.User
}

func gistInit(next Handler) Handler {
	return func(ctx *context.Context) error {
		currUser := ctx.User

		userName := ctx.Param("user")
		gistName := ctx.Param("gistname")

		switch filepath.Ext(gistName) {
		case ".js":
			ctx.SetData("gistpage", "js")
			gistName = strings.TrimSuffix(gistName, ".js")
		case ".json":
			ctx.SetData("gistpage", "json")
			gistName = strings.TrimSuffix(gistName, ".json")
		case ".git":
			ctx.SetData("gistpage", "git")
			gistName = strings.TrimSuffix(gistName, ".git")
		}

		gist, err := db.GetGist(userName, gistName)
		if err != nil {
			return ctx.NotFound("Gist not found")
		}

		// Expired gists are removed by a background job, but it may not have run
		// yet so hide them in the meantime so they're never served past expiry.
		if gist.IsExpired() {
			return ctx.NotFound("Gist not found")
		}

		if gist.Private == db.PrivateVisibility {
			if currUser == nil || currUser.ID != gist.UserID {
				// Check for token-based auth via Authorization header
				if tokenUser := getUserByToken(ctx); tokenUser != nil && tokenUser.ID == gist.UserID {
					// Token is valid and belongs to gist owner, allow access
				} else {
					return ctx.NotFound("Gist not found")
				}
			}
		}

		ctx.SetData("gist", gist)

		if ssh := gist.SSHCloneURL(ctx.Request().Host); ssh != "" {
			ctx.SetData("sshCloneUrl", ssh)
		}

		baseHttpUrl := ctx.GetData("baseHttpUrl").(string)

		if cloneURL := gist.HTTPCloneURL(baseHttpUrl); cloneURL != "" {
			ctx.SetData("httpCloneUrl", cloneURL)
		}

		ctx.SetData("httpCopyUrl", baseHttpUrl+"/"+userName+"/"+gistName)
		ctx.SetData("currentUrl", template.URL(ctx.Request().URL.Path))
		ctx.SetData("embedScript", fmt.Sprintf(`<script src="%s"></script>`, baseHttpUrl+"/"+userName+"/"+gistName+".js"))

		nbCommits, err := gist.NbCommits()
		if err != nil {
			return ctx.ErrorRes(500, "Error fetching number of commits", err)
		}
		ctx.SetData("nbCommits", nbCommits)

		if currUser != nil {
			hasLiked, err := currUser.HasLiked(gist)
			if err != nil {
				return ctx.ErrorRes(500, "Cannot get user like status", err)
			}
			ctx.SetData("hasLiked", hasLiked)
		}

		if gist.Private > 0 {
			ctx.SetData("NoIndex", true)
		}

		return next(ctx)
	}
}

// gistSoftInit try to load a gist (same as gistInit) but does not return a 404 if the gist is not found
// useful for git clients using HTTP to obfuscate the existence of a private gist.
// Access to the gist is checked later by the Git handler, so the gist is stored in the echo context
// instead of the template data, where an error page rendered for this request would display it.
func gistSoftInit(next Handler) Handler {
	return func(ctx *context.Context) error {
		userName := ctx.Param("user")
		gistName := ctx.Param("gistname")

		gistName = strings.TrimSuffix(gistName, ".git")

		gist, _ := db.GetGist(userName, gistName)
		ctx.Set("gist", gist)

		return next(ctx)
	}
}

// gistNewPushSoftInit has the same behavior as gistSoftInit but create a new gist empty instead
func gistNewPushSoftInit(next Handler) Handler {
	return func(ctx *context.Context) error {
		ctx.Set("gist", new(db.Gist))
		return next(ctx)
	}
}
func setAllGistsMode(mode string) Middleware {
	return func(next Handler) Handler {
		return func(ctx *context.Context) error {
			ctx.SetData("mode", mode)
			return next(ctx)
		}
	}
}

// apiBindAuth gates /api on the api.enabled config option and optionally
// resolves a caller identity from the Authorization header. A missing header is
// allowed (the downstream handler/scope middleware decides whether anonymous
// access is OK); a malformed/expired/unknown token is always rejected with 401.
// On success, sets ctx.User and stashes the access token under "accessToken".
func apiBindAuth(next Handler) Handler {
	return func(ctx *context.Context) error {
		if !config.C.ApiEnabled {
			return ctx.ErrorJson(403, "API is disabled", nil)
		}

		h := ctx.Request().Header.Get("Authorization")
		if h == "" {
			return next(ctx)
		}
		var plain string
		switch {
		case strings.HasPrefix(h, "Bearer "):
			plain = strings.TrimPrefix(h, "Bearer ")
		case strings.HasPrefix(h, "Token "):
			plain = strings.TrimPrefix(h, "Token ")
		default:
			return ctx.ErrorJson(401, "Bad credentials", nil)
		}

		tok, err := db.GetAccessTokenByToken(plain)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ctx.ErrorJson(401, "Bad credentials", nil)
			}
			return ctx.ErrorJson(500, "Error fetching access token", err)
		}
		if tok.IsExpired() {
			return ctx.ErrorJson(401, "Bad credentials", nil)
		}

		ctx.User = &tok.User
		ctx.SetData("accessToken", tok)

		// Synchronous update so that test teardown (TRUNCATE on postgres)
		// can't race with an in-flight UPDATE on a separate goroutine.
		// Cost is one indexed UPDATE per request; negligible vs the network
		// round-trip the caller already paid.
		_ = tok.UpdateLastUsed()

		return next(ctx)
	}
}

// apiCommentsEnabled is the /api counterpart of commentsEnabled, responding
// with a JSON 403 when comments are disabled in the config.
func apiCommentsEnabled(next Handler) Handler {
	return func(ctx *context.Context) error {
		if !config.C.CommentsEnabled {
			return ctx.ErrorJson(403, "Comments are disabled", nil)
		}
		return next(ctx)
	}
}

// apiRequireAuth rejects requests that apiAuth didn't attach a user to. Use on
// routes where anonymous access is not allowed (e.g. /user, write endpoints).
func apiRequireAuth(next Handler) Handler {
	return func(ctx *context.Context) error {
		if ctx.User == nil {
			return ctx.ErrorJson(401, "Requires authentication", nil)
		}
		return next(ctx)
	}
}

// ipcAuth guards the IPC API: only callers presenting the token derived from the
// secret key (i.e. QingyuGist's own subprocesses) may proceed.
func ipcAuth(next Handler) Handler {
	return func(ctx *context.Context) error {
		got := []byte(ctx.Request().Header.Get(ipc.AuthHeader))
		want := []byte(ipc.Token())
		if subtle.ConstantTimeCompare(got, want) != 1 {
			return ctx.NoContent(http.StatusUnauthorized)
		}
		return next(ctx)
	}
}

// dynamicCORS applies the instance CORS policy, which administrators can change
// at runtime without a restart. Only requests that carry an Origin header are
// affected, so same-origin browsing is untouched.
func dynamicCORS(next echo.HandlerFunc) echo.HandlerFunc {
	const exposed = "Link, X-Page, X-Per-Page, X-Total, X-Total-Pages"

	return func(c echo.Context) error {
		origin := c.Request().Header.Get("Origin")
		if origin == "" || !settings.CORSEnabled() {
			return next(c)
		}

		wildcard := false
		allowed := false
		for _, candidate := range settings.CORSOrigins() {
			if candidate == "*" {
				wildcard = true
				allowed = true
				break
			}
			if candidate == origin {
				allowed = true
			}
		}
		if !allowed {
			return next(c)
		}

		h := c.Response().Header()
		credentials := settings.CORSCredentials()
		if wildcard && !credentials {
			h.Set("Access-Control-Allow-Origin", "*")
		} else {
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
		}
		h.Set("Access-Control-Expose-Headers", exposed)

		if credentials {
			h.Set("Access-Control-Allow-Credentials", "true")
		}

		if c.Request().Method == http.MethodOptions && c.Request().Header.Get("Access-Control-Request-Method") != "" {
			h.Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
			if requested := c.Request().Header.Get("Access-Control-Request-Headers"); requested != "" {
				h.Set("Access-Control-Allow-Headers", requested)
			}
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			h.Set("Access-Control-Max-Age", "600")
			return c.NoContent(http.StatusNoContent)
		}

		return next(c)
	}
}

// redirectLegacy permanently redirects an old route to its replacement while
// preserving the query string. A 308 also preserves the request method and
// body, which matters for the handful of legacy POST endpoints.
func redirectLegacy(target string) Handler {
	return func(ctx *context.Context) error {
		url := target
		if q := ctx.Request().URL.RawQuery; q != "" {
			url += "?" + q
		}
		return ctx.Redirect(http.StatusPermanentRedirect, url)
	}
}

// redirectLegacyPrefix redirects every path below oldPrefix to the equivalent
// path below newPrefix, e.g. /settings/ssh -> /settings/ssh.
func redirectLegacyPrefix(oldPrefix, newPrefix string) Handler {
	return func(ctx *context.Context) error {
		url := newPrefix + strings.TrimPrefix(ctx.Request().URL.Path, oldPrefix)
		if q := ctx.Request().URL.RawQuery; q != "" {
			url += "?" + q
		}
		return ctx.Redirect(http.StatusPermanentRedirect, url)
	}
}

// redirectLegacyUserTab redirects /:user/-/liked to /:user/liked (and forked).
func redirectLegacyUserTab(tab string) Handler {
	return func(ctx *context.Context) error {
		url := "/" + ctx.Param("user") + "/" + tab
		if q := ctx.Request().URL.RawQuery; q != "" {
			url += "?" + q
		}
		return ctx.Redirect(http.StatusPermanentRedirect, url)
	}
}

// redirectReservedGist keeps gist URLs working for an account whose username
// is also a top-level route. Unknown paths under /admin (and the other
// user-facing route names) move to the collision-free /@username namespace.
func redirectReservedGist(name string) Handler {
	return func(ctx *context.Context) error {
		url := "/@" + name + strings.TrimPrefix(ctx.Request().URL.Path, "/"+name)
		if q := ctx.Request().URL.RawQuery; q != "" {
			url += "?" + q
		}
		return ctx.Redirect(http.StatusPermanentRedirect, url)
	}
}
