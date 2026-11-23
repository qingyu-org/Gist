package admin

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/qingyu-org/Gist/internal/actions"
	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/git"
	"github.com/qingyu-org/Gist/internal/index"
	"github.com/qingyu-org/Gist/internal/settings"
	qingyugistssh "github.com/qingyu-org/Gist/internal/ssh"
	"github.com/qingyu-org/Gist/internal/web/context"
	"github.com/qingyu-org/Gist/internal/web/handlers"
)

var processStart = time.Now()

func AdminIndex(ctx *context.Context) error {
	ctx.SetData("htmlTitle", ctx.TrH("admin.admin_panel"))
	ctx.SetData("adminHeaderPage", "index")

	ctx.SetData("qingyugistVersion", config.QingyuGistVersion)
	ctx.SetData("goVersion", runtime.Version())
	gitVersion, err := git.GetGitVersion()
	if err != nil {
		return ctx.ErrorRes(500, "Cannot get git version", err)
	}
	ctx.SetData("gitVersion", gitVersion)

	countUsers, err := db.CountAll(&db.User{})
	if err != nil {
		return ctx.ErrorRes(500, "Cannot count users", err)
	}
	ctx.SetData("countUsers", countUsers)

	countGists, err := db.CountAll(&db.Gist{})
	if err != nil {
		return ctx.ErrorRes(500, "Cannot count gists", err)
	}
	ctx.SetData("countGists", countGists)

	publicGists, _ := db.CountGistsByVisibility(db.PublicVisibility)
	privateGists, _ := db.CountGistsByVisibility(db.PrivateVisibility)
	ctx.SetData("countPublicGists", publicGists)
	ctx.SetData("countPrivateGists", privateGists)

	countKeys, err := db.CountAll(&db.SSHKey{})
	if err != nil {
		return ctx.ErrorRes(500, "Cannot count SSH keys", err)
	}
	ctx.SetData("countKeys", countKeys)
	countInvites, _ := db.CountAll(&db.Invitation{})
	ctx.SetData("countInvitations", countInvites)

	home := config.GetHomeDir()
	storageRepos := dirSize(filepath.Join(home, "repos"))
	storageUploads := dirSize(filepath.Join(home, "uploads"))
	storageIndex := dirSize(filepath.Join(home, "qingyugist.index"))
	ctx.SetData("storageRepos", storageRepos)
	ctx.SetData("storageUploads", storageUploads)
	ctx.SetData("storageIndex", storageIndex)
	// Content storage, not the whole data directory: the latter also contains
	// the database, logs, sessions and (on Windows) a copy of the executable,
	// which would make the number look alarming and unactionable.
	ctx.SetData("storageTotal", storageRepos+storageUploads+storageIndex)

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	ctx.SetData("memAlloc", mem.Alloc)
	ctx.SetData("memSys", mem.Sys)
	ctx.SetData("memHeap", mem.HeapAlloc)
	ctx.SetData("goroutines", runtime.NumGoroutine())
	ctx.SetData("uptime", time.Since(processStart))
	ctx.SetData("indexEnabled", index.IndexEnabled())
	ctx.SetData("indexType", index.IndexType())
	ctx.SetData("lastIndexRun", settings.LastIndexRun())

	// The legacy admin dashboard renders actions on this page. Keep supplying
	// their state while the new UI uses the dedicated actions page.
	ctx.SetData("syncReposFromFS", actions.IsRunning(actions.SyncReposFromFS))
	ctx.SetData("syncReposFromDB", actions.IsRunning(actions.SyncReposFromDB))
	ctx.SetData("gitGcRepos", actions.IsRunning(actions.GitGcRepos))
	ctx.SetData("syncGistPreviews", actions.IsRunning(actions.SyncGistPreviews))
	ctx.SetData("resetHooks", actions.IsRunning(actions.ResetHooks))
	ctx.SetData("indexGists", actions.IsRunning(actions.IndexGists))
	ctx.SetData("syncGistLanguages", actions.IsRunning(actions.SyncGistLanguages))
	ctx.SetData("deleteExpiredGists", actions.IsRunning(actions.DeleteExpiredGists))
	ctx.SetData("syncSSHKeys", actions.IsRunning(actions.SyncSSHKeys))
	ctx.SetData("sshManagesAuthorizedKeys", config.C.SshManagesAuthorizedKeys())
	return ctx.Html("admin_index.html")
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func AdminUsers(ctx *context.Context) error {
	ctx.SetData("htmlTitle", ctx.TrH("admin.users")+" - "+ctx.TrH("admin.admin_panel"))
	ctx.SetData("adminHeaderPage", "users")
	ctx.SetData("loadStartTime", time.Now())

	pageInt := handlers.GetPage(ctx)

	var data []*db.User
	var err error
	if data, err = db.GetAllUsers(pageInt - 1); err != nil {
		return ctx.ErrorRes(500, "Cannot get users", err)
	}

	if err = handlers.Paginate(ctx, data, pageInt, 10, "data", "admin/users", 1, nil); err != nil {
		return ctx.ErrorRes(404, ctx.Tr("error.page-not-found"), nil)
	}

	return ctx.Html("admin_users.html")
}

func AdminGists(ctx *context.Context) error {
	ctx.SetData("htmlTitle", ctx.TrH("admin.gists")+" - "+ctx.TrH("admin.admin_panel"))
	ctx.SetData("adminHeaderPage", "gists")
	pageInt := handlers.GetPage(ctx)

	var data []*db.Gist
	var err error
	if data, err = db.GetAllGists(pageInt - 1); err != nil {
		return ctx.ErrorRes(500, "Cannot get gists", err)
	}

	if err = handlers.Paginate(ctx, data, pageInt, 10, "data", "admin/gists", 1, nil); err != nil {
		return ctx.ErrorRes(404, ctx.Tr("error.page-not-found"), nil)
	}

	return ctx.Html("admin_gists.html")
}

func AdminUserDelete(ctx *context.Context) error {
	userId, _ := strconv.ParseUint(ctx.Param("user"), 10, 64)
	user, err := db.GetUserById(uint(userId))
	if err != nil {
		return ctx.ErrorRes(500, "Cannot retrieve user", err)
	}

	// Deleting the account the request is made with would sign the administrator
	// out mid-action and can leave the instance without an admin.
	if user.ID == ctx.User.ID {
		ctx.AddFlash(ctx.Tr("flash.admin.cannot-delete-self"), "error")
		return ctx.RedirectTo("/admin/users")
	}

	if err := user.Delete(); err != nil {
		return ctx.ErrorRes(500, "Cannot delete this user", err)
	}
	qingyugistssh.SyncAuthorizedKeysLogged()

	ctx.AddFlash(ctx.Tr("flash.admin.user-deleted"), "success")
	return ctx.RedirectTo("/admin/users")
}

func AdminGistDelete(ctx *context.Context) error {
	gist, err := db.GetGistByID(ctx.Param("gist"))
	if err != nil {
		return ctx.ErrorRes(500, "Cannot retrieve gist", err)
	}

	if err = gist.Delete(); err != nil {
		return ctx.ErrorRes(500, "Cannot delete this gist", err)
	}

	gist.RemoveFromIndex()

	ctx.AddFlash(ctx.Tr("flash.admin.gist-deleted"), "success")
	return ctx.RedirectTo("/admin/gists")
}

type configSection struct {
	Key    string
	Label  string
	Icon   string
	Fields []settings.Definition
	// Tabs is set instead of Fields for the third-party sign-in section: one
	// panel per provider, shown one at a time behind a tab bar.
	Tabs []configOAuthTab
}

// configOAuthTab is one provider of the third-party sign-in section.
type configOAuthTab struct {
	Key     string
	Label   string // i18n key of the tab label
	Enabled bool
	Fields  []settings.Definition
}

// oauthTabs groups the third-party definitions by provider, keeping the
// declaration order of settings/oauth.go.
func oauthTabs(values map[string]string) []configOAuthTab {
	tabs := make([]configOAuthTab, 0, 6)
	index := make(map[string]int, 6)
	for _, d := range settings.DefinitionsByCategory(settings.CategoryOAuth) {
		i, ok := index[d.Provider]
		if !ok {
			i = len(tabs)
			index[d.Provider] = i
			tabs = append(tabs, configOAuthTab{Key: d.Provider, Label: "settings.oauth.tab_" + d.Provider})
		}
		if settings.IsOAuthEnabledKey(d.Key) {
			tabs[i].Enabled = values[d.Key] == "1"
		}
		tabs[i].Fields = append(tabs[i].Fields, d)
	}
	return tabs
}

var configSectionMeta = map[settings.Category]struct {
	label string
	icon  string
}{
	settings.CategorySite:       {"settings.category.site", "layout-template"},
	settings.CategoryAuth:       {"settings.category.auth", "key-round"},
	settings.CategoryOAuth:      {"settings.category.oauth", "log-in"},
	settings.CategoryLDAP:       {"settings.category.ldap", "users"},
	settings.CategoryUpload:     {"settings.category.upload", "upload"},
	settings.CategoryComments:   {"settings.category.comments", "message-square"},
	settings.CategorySearch:     {"settings.category.search", "search"},
	settings.CategorySecurity:   {"settings.category.security", "shield-check"},
	settings.CategoryModeration: {"settings.category.moderation", "sparkles"},
	settings.CategoryGit:        {"settings.category.git", "git-branch"},
	settings.CategoryAPI:        {"settings.category.api", "plug"},
}

func adminConfigData(ctx *context.Context, values map[string]string) {
	ctx.SetData("htmlTitle", ctx.TrH("admin.configuration")+" - "+ctx.TrH("admin.admin_panel"))
	ctx.SetData("adminHeaderPage", "config")

	sections := make([]configSection, 0)
	for _, category := range settings.Categories() {
		meta := configSectionMeta[category]
		section := configSection{
			Key:    string(category),
			Label:  meta.label,
			Icon:   meta.icon,
			Fields: settings.DefinitionsByCategory(category),
		}
		if category == settings.CategoryOAuth {
			section.Fields = nil
			section.Tabs = oauthTabs(values)
		}
		sections = append(sections, section)
	}
	ctx.SetData("settingSections", sections)
	ctx.SetData("settingValues", values)
	ctx.SetData("secretPlaceholder", settings.SecretPlaceholder)
	if links, err := settings.ParseSiteLinks(values[settings.KeySiteLinks]); err == nil {
		ctx.SetData("SiteLinks", links)
	}

	ctx.SetData("dbtype", db.DatabaseInfo.Type.String())
	ctx.SetData("dbname", db.DatabaseInfo.Database)
	ctx.SetData("indexEnabled", index.IndexEnabled())
	ctx.SetData("indexType", index.IndexType())
	ctx.SetData("lastIndexRun", settings.LastIndexRun())
	ctx.SetData("config", config.C)
}

func AdminConfig(ctx *context.Context) error {
	values := settings.Snapshot()
	for _, d := range settings.AllDefinitions() {
		if d.Kind != settings.KindSecret {
			continue
		}
		if values[d.Key] != "" {
			values[d.Key] = settings.SecretPlaceholder
		}
	}
	adminConfigData(ctx, values)
	return ctx.Html("admin_config.html")
}

// AdminUpdateConfig saves the whole configuration form. It validates every
// field first so a single bad value does not leave the instance half-updated.
func AdminUpdateConfig(ctx *context.Context) error {
	values := make(map[string]string, len(settings.AllDefinitions()))
	var errs []string

	for _, d := range settings.AllDefinitions() {
		var value string
		if d.Kind == settings.KindBool {
			value = "0"
			if v := ctx.FormValue(d.Key); v == "1" || v == "on" || v == "true" {
				value = "1"
			}
		} else {
			value = strings.TrimSpace(ctx.FormValue(d.Key))
		}

		if d.Kind == settings.KindSecret {
			// An untouched masked field keeps the stored secret.
			if value == "" || value == settings.SecretPlaceholder {
				continue
			}
		}

		if err := settings.Validate(d.Key, value); err != nil {
			errs = append(errs, ctx.Tr(d.Label)+": "+err.Error())
			continue
		}
		values[d.Key] = value
	}

	if len(errs) > 0 {
		for _, d := range settings.AllDefinitions() {
			if _, ok := values[d.Key]; ok {
				continue
			}
			if d.Kind == settings.KindSecret {
				if v := ctx.FormValue(d.Key); v != "" {
					values[d.Key] = v
				}
				continue
			}
			values[d.Key] = ctx.FormValue(d.Key)
		}
		ctx.AddFlash(strings.Join(errs, " · "), "error")
		adminConfigData(ctx, values)
		return ctx.Html("admin_config.html")
	}

	if err := settings.SetMany(values); err != nil {
		ctx.AddFlash(ctx.Tr("flash.admin.config-save-failed")+": "+err.Error(), "error")
		adminConfigData(ctx, settings.Snapshot())
		return ctx.Html("admin_config.html")
	}

	ctx.AddFlash(ctx.Tr("flash.admin.config-saved"), "success")
	return ctx.RedirectTo("/admin/settings")
}

// AdminSetConfig is the legacy endpoint used by the old dashboard switches. It
// still accepts the legacy dash keys and forwards known keys to the registry.
func AdminSetConfig(ctx *context.Context) error {
	key := ctx.FormValue("key")
	value := ctx.FormValue("value")
	key, value = settings.NormalizeLegacyValue(key, value)

	if _, ok := settings.Define(key); !ok {
		// Only registry keys are accepted: an unknown key would be stored
		// unvalidated and could surface as a stale value under a future setting.
		return ctx.ErrorRes(400, "Unknown setting", nil)
	}
	if err := settings.Set(key, value); err != nil {
		return ctx.ErrorRes(400, "Invalid setting", err)
	}

	return ctx.JSON(200, map[string]interface{}{
		"success": true,
	})
}

func AdminInvitations(ctx *context.Context) error {
	ctx.SetData("htmlTitle", ctx.TrH("admin.invitations")+" - "+ctx.TrH("admin.admin_panel"))
	ctx.SetData("adminHeaderPage", "invitations")

	var invitations []*db.Invitation
	var err error
	if invitations, err = db.GetAllInvitations(); err != nil {
		return ctx.ErrorRes(500, "Cannot get invites", err)
	}

	ctx.SetData("invitations", invitations)
	return ctx.Html("admin_invitations.html")
}

func AdminInvitationsCreate(ctx *context.Context) error {
	code := ctx.FormValue("code")
	nbMax, err := strconv.ParseUint(ctx.FormValue("nbMax"), 10, 64)
	if err != nil {
		nbMax = 10
	}

	// The form sends a datetime-local value (no timezone, so read it as local
	// time); the legacy dashboard sent a Unix timestamp. Empty means one week.
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	if raw := ctx.FormValue("expiresAt"); raw != "" {
		parsed, parseErr := time.ParseInLocation("2006-01-02T15:04", raw, time.Local)
		if parseErr != nil {
			// Browsers include seconds when the input has a step finer than a minute.
			parsed, parseErr = time.ParseInLocation("2006-01-02T15:04:05", raw, time.Local)
		}
		if parseErr != nil {
			return ctx.ErrorRes(400, "Invalid expiration date", parseErr)
		}
		expiresAt = parsed
	} else if raw := ctx.FormValue("expiredAtUnix"); raw != "" {
		unix, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil {
			return ctx.ErrorRes(400, "Invalid expiration date", parseErr)
		}
		expiresAt = time.Unix(unix, 0)
	}

	invitation := &db.Invitation{
		Code:      code,
		ExpiresAt: expiresAt.Unix(),
		NbMax:     uint(nbMax),
	}

	if err := invitation.Create(); err != nil {
		return ctx.ErrorRes(500, "Cannot create invitation", err)
	}

	ctx.AddFlash(ctx.Tr("flash.admin.invitation-created"), "success")
	return ctx.RedirectTo("/admin/invitations")
}

func AdminInvitationsDelete(ctx *context.Context) error {
	id, _ := strconv.ParseUint(ctx.Param("id"), 10, 64)
	invitation, err := db.GetInvitationByID(uint(id))
	if err != nil {
		return ctx.ErrorRes(500, "Cannot retrieve invitation", err)
	}

	if err := invitation.Delete(); err != nil {
		return ctx.ErrorRes(500, "Cannot delete this invitation", err)
	}

	ctx.AddFlash(ctx.Tr("flash.admin.invitation-deleted"), "success")
	return ctx.RedirectTo("/admin/invitations")
}
