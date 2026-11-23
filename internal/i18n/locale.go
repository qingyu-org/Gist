package i18n

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qingyu-org/Gist/internal/i18n/locales"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
	"gopkg.in/yaml.v3"
)

var Locales = NewLocaleStore()

// DefaultLocaleCode is the language every missing message falls back to.
const DefaultLocaleCode = "en-US"

// CompleteCoverage is the share of en-US messages a locale needs before it is
// offered to visitors. A half-translated locale renders as a mix of two
// languages, which reads worse than plain English, so partial translations
// stay in the repository unused until they are finished.
const CompleteCoverage = 0.95

type LocaleStore struct {
	Locales map[string]*Locale

	// offered holds the complete locales, sorted by code. offeredCodes and
	// offeredTags are parallel slices indexed by the matcher, built once in
	// LoadAll.
	offered      map[string]*Locale
	offeredCodes []string
	offeredTags  []language.Tag
	matcher      language.Matcher
}

type Locale struct {
	Code     string
	Name     string
	Messages map[string]string

	// Coverage is the share of en-US messages this locale defines, from 0 to 1.
	Coverage float64
	// Complete reports whether the locale is finished enough to be offered.
	Complete bool
}

// NewLocaleStore creates a new LocaleStore
func NewLocaleStore() *LocaleStore {
	return &LocaleStore{
		Locales: make(map[string]*Locale),
	}
}

// loadLocaleFromYAML loads a single Locale from a given YAML file
func (store *LocaleStore) loadLocaleFromYAML(localeCode, path string) error {
	a, err := locales.Files.Open(path)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(a)
	if err != nil {
		return err
	}

	tag, err := language.Parse(localeCode)
	if err != nil {
		return err
	}

	name := display.Self.Name(tag)
	switch tag {
	case language.AmericanEnglish:
		name = "English"
	case language.EuropeanSpanish:
		name = "Español"
	}
	switch localeCode {
	case "zh-CN":
		name = "简体中文"
	case "zh-TW":
		name = "繁體中文"
	}

	locale := &Locale{
		Code:     localeCode,
		Name:     cases.Title(language.English).String(name),
		Messages: make(map[string]string),
	}

	err = yaml.Unmarshal(data, &locale.Messages)
	if err != nil {
		return err
	}

	store.Locales[localeCode] = locale
	return nil
}

func (store *LocaleStore) LoadAll() error {
	err := fs.WalkDir(locales.Files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			localeKey := strings.TrimSuffix(path, filepath.Ext(path))
			err := store.loadLocaleFromYAML(localeKey, path)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	store.markCoverage()
	return nil
}

// markCoverage scores every locale against en-US and decides which ones the
// language switcher and Accept-Language negotiation may use.
func (store *LocaleStore) markCoverage() {
	reference := store.Locales[DefaultLocaleCode]
	referenceKeys := 0
	if reference != nil {
		referenceKeys = len(reference.Messages)
	}

	store.offered = make(map[string]*Locale)
	store.offeredCodes = nil
	store.offeredTags = nil

	codes := make([]string, 0, len(store.Locales))
	for code := range store.Locales {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	for _, code := range codes {
		locale := store.Locales[code]
		if referenceKeys == 0 {
			locale.Coverage = 1
		} else {
			translated := 0
			for key := range reference.Messages {
				if locale.Messages[key] != "" {
					translated++
				}
			}
			locale.Coverage = float64(translated) / float64(referenceKeys)
		}

		locale.Complete = locale.Coverage >= CompleteCoverage
		if locale.Complete {
			store.offered[code] = locale
			store.offeredCodes = append(store.offeredCodes, code)
			store.offeredTags = append(store.offeredTags, locale.tag())
		}
	}

	if _, ok := store.offered[DefaultLocaleCode]; !ok && store.Locales[DefaultLocaleCode] != nil {
		// The default has to be usable even if it somehow fell below the bar.
		store.offered[DefaultLocaleCode] = store.Locales[DefaultLocaleCode]
		store.offeredCodes = append(store.offeredCodes, DefaultLocaleCode)
		store.offeredTags = append(store.offeredTags, language.AmericanEnglish)
	}

	store.matcher = nil
	if len(store.offeredTags) > 0 {
		store.matcher = language.NewMatcher(store.offeredTags)
	}
}

// tag turns the locale code into a language tag for matching. Codes that
// x/text would normalize into something else (it would read "uk-UK" as a
// British-region tag) are corrected by naming the file after the real region.
func (l *Locale) tag() language.Tag {
	tag, err := language.Parse(l.Code)
	if err != nil {
		return language.English
	}
	return tag
}

// Offered returns the complete locales in menu order.
func (store *LocaleStore) Offered() map[string]*Locale {
	if store.offered == nil {
		return store.Locales
	}
	return store.offered
}

func (store *LocaleStore) GetLocale(lang string) (*Locale, error) {
	_, ok := store.Locales[lang]
	if !ok {
		return nil, fmt.Errorf("locale %s not found", lang)
	}

	return store.Locales[lang], nil
}

// Usable reports whether lang may be selected: it exists and is complete.
func (store *LocaleStore) Usable(lang string) bool {
	locale, ok := store.Locales[lang]
	return ok && locale.Complete
}

// MatchTag picks the complete locale that best fits the browser's preferences.
// It matches on base languages too, so a browser asking for "ja" or "de-AT"
// resolves instead of silently falling back to English, and a region-less
// "zh" resolves to whichever Chinese locale is offered.
func (store *LocaleStore) MatchTag(langs []language.Tag) string {
	if store.matcher == nil || len(langs) == 0 {
		return DefaultLocaleCode
	}

	_, index, confidence := store.matcher.Match(langs...)
	if confidence == language.No || index < 0 || index >= len(store.offeredCodes) {
		// No is what the matcher returns when it fell back to its first tag
		// without understanding anything the browser asked for.
		return DefaultLocaleCode
	}

	return store.offeredCodes[index]
}

func (l *Locale) String(key string, args ...any) string {
	message := l.Messages[key]

	if message == "" {
		fallback := Locales.Locales["en-US"]
		if fallback == nil || fallback.Code == l.Code {
			return key
		}
		return fallback.String(key, args...)
	}

	if len(args) == 0 {
		return message
	}

	return fmt.Sprintf(message, args...)
}

func (l *Locale) Tr(key string, args ...any) template.HTML {
	message := l.Messages[key]

	if message == "" {
		fallback := Locales.Locales["en-US"]
		if fallback == nil || fallback.Code == l.Code {
			return template.HTML(key)
		}
		return fallback.Tr(key, args...)
	}

	if len(args) == 0 {
		return template.HTML(message)
	}

	return template.HTML(fmt.Sprintf(message, args...))
}

// TimeAgo renders a Unix timestamp as a localized, approximate relative time
// ("3 minutes ago", "2 个月前"). Using the request locale keeps dates from
// falling back to English halfway through an otherwise translated page.
func (l *Locale) TimeAgo(timestamp int64) template.HTML {
	return l.timeAgo(time.Unix(timestamp, 0), time.Now())
}

// TimeAgoStr is the string-timestamp variant used by git revision metadata.
func (l *Locale) TimeAgoStr(timestamp string) template.HTML {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return l.Tr("time.just-now")
	}
	return l.TimeAgo(seconds)
}

func (l *Locale) timeAgo(t, now time.Time) template.HTML {
	diff := now.Sub(t)
	if diff < 10*time.Second {
		return l.Tr("time.just-now")
	}

	unit := func(key string, value int) template.HTML {
		if value == 1 {
			singular := strings.TrimSuffix(key, "s")
			if message := l.Messages[singular]; message != "" {
				return template.HTML(message)
			}
		}
		return l.Tr(key, value)
	}

	switch {
	case diff < time.Minute:
		return unit("time.seconds", int(diff/time.Second))
	case diff < time.Hour:
		return unit("time.minutes", int(diff/time.Minute))
	case diff < 24*time.Hour:
		return unit("time.hours", int(diff/time.Hour))
	case diff < 30*24*time.Hour:
		return unit("time.days", int(diff/(24*time.Hour)))
	case diff < 365*24*time.Hour:
		return unit("time.months", int(diff/(30*24*time.Hour)))
	default:
		return unit("time.years", int(diff/(365*24*time.Hour)))
	}
}
