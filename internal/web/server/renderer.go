package server

import (
	gojson "encoding/json"
	"errors"
	"fmt"
	htmlpkg "html"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/db"
	"github.com/qingyu-org/Gist/internal/index"
	"github.com/qingyu-org/Gist/internal/settings"
	"github.com/qingyu-org/Gist/internal/validator"
	"github.com/qingyu-org/Gist/internal/web/context"
	"github.com/qingyu-org/Gist/internal/web/handlers"
	"github.com/qingyu-org/Gist/public"
	"github.com/qingyu-org/Gist/templates"
	"github.com/rs/zerolog/log"
)

type Template struct {
	templates *template.Template
	// pages holds layout-based templates, keyed by file name (e.g. "all.html").
	// Each entry is a clone of the base layout with that page's "content" block parsed in.
	pages map[string]*template.Template
	// customPages holds the file names of the templates parsed from $QYG_HOME/custom.
	customPages map[string]bool
}

// customPagePrefix marks a template name passed to Render as a file from
// $QYG_HOME/custom (e.g. "custom/legal.html"). Such a name only resolves to
// that file, never to a built-in page, layout or partial.
const customPagePrefix = "custom/"

func (t *Template) Render(w io.Writer, name string, data interface{}, _ echo.Context) error {
	if customName, ok := strings.CutPrefix(name, customPagePrefix); ok {
		if !t.customPages[customName] {
			return fmt.Errorf("custom template %q not found", customName)
		}
		return t.templates.ExecuteTemplate(w, customName, data)
	}
	if tmpl, ok := t.pages[name]; ok {
		return tmpl.ExecuteTemplate(w, "base", data)
	}
	return t.templates.ExecuteTemplate(w, name, data)
}

var re = regexp.MustCompile("[^a-z0-9]+")

func (s *Server) setFuncMap() {
	fm := template.FuncMap{
		"split":     strings.Split,
		"indexByte": strings.IndexByte,
		"toInt": func(i string) int {
			val, _ := strconv.Atoi(i)
			return val
		},
		"inc": func(i int) int {
			return i + 1
		},
		"splitGit": func(i string) []string {
			return strings.FieldsFunc(i, func(r rune) bool {
				return r == ',' || r == ' '
			})
		},
		"lines": func(i string) []string {
			return strings.Split(i, "\n")
		},
		"isMarkdown": func(i string) bool {
			return strings.ToLower(filepath.Ext(i)) == ".md"
		},
		"isMermaid": func(i string) bool {
			return strings.ToLower(filepath.Ext(i)) == ".mmd"
		},
		"isJupyter": func(i string) bool {
			return strings.ToLower(filepath.Ext(i)) == ".ipynb"
		},
		"httpStatusText": http.StatusText,
		"loadedTime": func(startTime time.Time) string {
			return fmt.Sprint(time.Since(startTime).Nanoseconds()/1e6) + "ms"
		},
		"slug": func(s string) string {
			return strings.Trim(re.ReplaceAllString(strings.ToLower(s), "-"), "-")
		},
		// Avatars are never stored: they are derived from the account's email
		// address through the configured Gravatar-style provider. The uploaded
		// file and provider-CDN picture that older releases wrote into
		// user.AvatarURL are no longer used.
		"avatarUrl": func(user *db.User) string {
			if user == nil {
				return ""
			}
			return settings.AvatarURL(user.MD5Hash)
		},
		// True when the caller should draw an identicon instead of loading an
		// image: no provider, no email hash, and no fallback picture.
		"shouldGenerateAvatar": func(user *db.User) bool {
			if user == nil {
				return true
			}
			return settings.AvatarURL(user.MD5Hash) == ""
		},
		"profilePath": validator.ProfilePath,
		"asset": func(file string) string {
			if s.dev {
				return "http://localhost:16157/" + file
			}
			return config.C.ExternalUrl + "/" + context.ManifestEntries[file].File
		},
		"assetCss": func(file string) string {
			if s.dev {
				return "http://localhost:16157/" + file
			}
			return config.C.ExternalUrl + "/" + context.ManifestEntries[file].Css[0]
		},
		"custom": func(file string) string {
			file = strings.TrimSpace(file)
			// A logo/favicon may point at an external image just as well as a
			// file dropped into <home>/custom.
			if strings.HasPrefix(file, "http://") || strings.HasPrefix(file, "https://") || strings.HasPrefix(file, "//") {
				return file
			}
			assetpath, err := url.JoinPath("/", "assets", strings.TrimPrefix(file, "/"))
			if err != nil {
				log.Error().Err(err).Msgf("Failed to join path for custom file %s", file)
			}
			return config.C.ExternalUrl + assetpath
		},
		"dev": func() bool {
			return s.dev
		},
		"visibilityStr": func(visibility db.Visibility, lowercase bool) string {
			s := "Public"
			switch visibility {
			case 1:
				s = "Unlisted"
			case 2:
				s = "Private"
			}

			if lowercase {
				return strings.ToLower(s)
			}
			return s
		},
		"unescape": htmlpkg.UnescapeString,
		"join": func(s ...string) string {
			return strings.Join(s, "")
		},
		"toStr": func(i interface{}) string {
			return fmt.Sprint(i)
		},
		"safe": func(s string) template.HTML {
			return template.HTML(s)
		},
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, errors.New("invalid dict call")
			}
			dict := make(map[string]interface{})
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, errors.New("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"addMetadataToSearchQuery": func(input, key, value string) string {
			metadata := handlers.ParseSearchQueryStr(input)
			// extract free-text content (stored under "all") and remove it from metadata
			content := metadata["all"]
			delete(metadata, "all")

			metadata[key] = value

			var resultBuilder strings.Builder
			resultBuilder.WriteString(content)

			for k, v := range metadata {
				resultBuilder.WriteString(" ")
				resultBuilder.WriteString(k)
				resultBuilder.WriteString(":")
				resultBuilder.WriteString(v)
			}

			return strings.TrimSpace(resultBuilder.String())
		},
		"indexEnabled": index.IndexEnabled,
		"isUrl": func(s string) bool {
			_, err := url.ParseRequestURI(s)
			return err == nil
		},
		"topicsToStr": func(topics []db.GistTopic) string {
			str := ""
			for i, topic := range topics {
				if i > 0 {
					str += " "
				}
				str += topic.Topic
			}
			return str
		},
		// ogText collapses all whitespace (newlines, tabs, multiple spaces) into
		// single spaces and truncates to maxRunes runes, appending an ellipsis when
		// truncated. Used to build clean values for Open Graph / Twitter meta tags
		// from multi-line gist descriptions or code previews.
		"ogText": func(s string, maxRunes int) string {
			collapsed := strings.Join(strings.Fields(s), " ")
			runes := []rune(collapsed)
			if len(runes) <= maxRunes {
				return collapsed
			}
			return string(runes[:maxRunes]) + "\u2026"
		},
		"hexToRgb": func(hex string) string {
			h, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
			return fmt.Sprintf("%d, %d, %d,", (h>>16)&0xFF, (h>>8)&0xFF, h&0xFF)
		},
		"humanDate": func(t int64) string {
			return time.Unix(t, 0).Format("2006-01-02 15:04")
		},
		"humanDateOnly": func(t int64) string {
			return time.Unix(t, 0).Format("2006-01-02")
		},
		"humanSize":     humanSize,
		"humanDuration": humanDuration,
		"currentYear": func() int {
			return time.Now().Year()
		},
		"percent": func(part, total int64) int {
			if total <= 0 {
				return 0
			}
			return int(float64(part) / float64(total) * 100)
		},
		// withQuery returns the current request URI with one query parameter
		// replaced. Used by the sidebar language switcher so changing language
		// keeps the active filters and pagination.
		"withQuery": func(requestURI, key, value string) string {
			u, err := url.Parse(requestURI)
			if err != nil {
				return requestURI
			}
			q := u.Query()
			q.Set(key, value)
			u.RawQuery = q.Encode()
			return u.String()
		},
		"mainTheme": func(theme *db.UserStyleDTO) string {
			if theme == nil {
				return "auto"
			}

			if theme.Theme == "" {
				return "auto"
			}

			return theme.Theme
		},
	}

	base := template.Must(template.New("base").Funcs(fm).ParseFS(templates.Files, "layouts/*.html", "partials/*.html"))
	pagePaths, err := fs.Glob(templates.Files, "pages/*.html")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to glob new page templates")
	}
	pages := make(map[string]*template.Template, len(pagePaths))
	for _, p := range pagePaths {
		cloned := template.Must(base.Clone())
		pages[filepath.Base(p)] = template.Must(cloned.ParseFS(templates.Files, p))
	}

	customTemplates := template.Must(base.Clone())
	customPattern := filepath.Join(config.GetHomeDir(), "custom", "*.html")
	matches, err := filepath.Glob(customPattern)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to check for custom templates")
	}
	customPages := make(map[string]bool, len(matches))
	if len(matches) > 0 {
		customTemplates, err = customTemplates.ParseFiles(matches...)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to parse custom templates")
		}
		for _, m := range matches {
			customPages[filepath.Base(m)] = true
		}
	}

	s.echo.Renderer = &Template{
		templates:   customTemplates,
		pages:       pages,
		customPages: customPages,
	}
}

func (s *Server) parseManifestEntries() {
	entries, err := parseManifest(public.Files)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load manifest.json")
	}
	context.ManifestEntries = entries
}

func parseManifest(files fs.FS) (map[string]context.Asset, error) {
	file, err := files.Open(".vite/manifest.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	byteValue, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	entries := make(map[string]context.Asset)
	if err = gojson.Unmarshal(byteValue, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// humanSize formats a byte count the way a storage dashboard should read:
// compact, one decimal at most, and never scientific.
func humanSize(value any) string {
	var bytes int64
	switch v := value.(type) {
	case int64:
		bytes = v
	case int:
		bytes = int64(v)
	case uint64:
		bytes = int64(v)
	case uint:
		bytes = int64(v)
	case float64:
		bytes = int64(v)
	default:
		return "0 B"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// humanDuration renders an uptime without the sub-second noise of Duration.String.
func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
