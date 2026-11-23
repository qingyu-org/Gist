package server_test

import (
	"strings"
	"testing"

	"github.com/qingyu-org/Gist/internal/i18n"
	"github.com/qingyu-org/Gist/internal/web/test"
	"github.com/stretchr/testify/require"
)

func TestLocaleFromAcceptLanguage(t *testing.T) {
	s := test.Setup(t)
	defer test.Teardown(t)

	localeFor := func(acceptLanguage string) string {
		s.RequestWithHeaders(t, "GET", "/login", nil, 200, map[string]string{"Accept-Language": acceptLanguage})
		locale, ok := s.ContextData("locale")
		require.True(t, ok)
		return locale.(*i18n.Locale).Code
	}

	// Complete locales are honoured, matching on the base language too.
	require.Equal(t, "zh-CN", localeFor("zh-CN,zh;q=0.9,en;q=0.8"))
	require.Equal(t, "zh-CN", localeFor("zh-Hans"))
	// fr-FR exists but is only a third translated, so asking for it lands on
	// English rather than on a half-translated page.
	require.Equal(t, "en-US", localeFor("fr-FR,fr;q=0.9,en;q=0.8"))
	require.Equal(t, "en-US", localeFor(""))

	// Entries past the first 512 bytes are ignored, even zh-CN with the highest
	// weight.
	require.Equal(t, "en-US", localeFor(strings.Repeat("aa;q=0.1,", 60)+"zh-CN"))
}
