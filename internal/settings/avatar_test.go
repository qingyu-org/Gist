package settings

import (
	"net/url"
	"strings"
	"testing"

	"github.com/qingyu-org/Gist/internal/db"
	"github.com/stretchr/testify/require"
)

func TestAvatarURLForProvider(t *testing.T) {
	hash := strings.Repeat("a", 32)

	require.Equal(t,
		"https://weavatar.com/avatar/"+hash+"?s=200&d=identicon",
		avatarURL(AvatarProviderWeAvatar, "", hash))

	require.Equal(t,
		"https://www.gravatar.com/avatar/"+hash+"?s=200&d=identicon",
		avatarURL(AvatarProviderGravatar, "", hash))
}

func TestAvatarURLFallback(t *testing.T) {
	hash := strings.Repeat("a", 32)
	fallback := "https://cdn.example.com/avatar.png"

	got := avatarURL(AvatarProviderWeAvatar, fallback, hash)
	require.Contains(t, got, "d="+url.QueryEscape(fallback))

	// A keyword such as mp or identicon is part of the protocol, not a URL, and
	// must not be escaped.
	require.Equal(t,
		"https://weavatar.com/avatar/"+hash+"?s=200&d=mp",
		avatarURL(AvatarProviderWeAvatar, "mp", hash))

	// Whitespace around a pasted URL never reaches the query string.
	require.Equal(t,
		"https://weavatar.com/avatar/"+hash+"?s=200&d=mp",
		avatarURL(AvatarProviderWeAvatar, "  mp  ", hash))
}

func TestAvatarURLWithoutProviderOrHash(t *testing.T) {
	fallback := "https://cdn.example.com/default.png"

	require.Equal(t, fallback, avatarURL(AvatarProviderNone, fallback, strings.Repeat("a", 32)),
		"generated avatars keep the configured default picture")
	require.Equal(t, "", avatarURL(AvatarProviderNone, "", strings.Repeat("a", 32)))
	require.Equal(t, fallback, avatarURL(AvatarProviderWeAvatar, fallback, ""),
		"an account without an email cannot be looked up")
	require.Equal(t, "", avatarURL(AvatarProviderWeAvatar, "", ""))
}

func TestAvatarProviderDefault(t *testing.T) {
	// init() installs an empty value map; an unset key must still resolve to a
	// working provider rather than an empty host.
	require.Equal(t, AvatarProviderWeAvatar, AvatarProvider())
	require.Equal(t, "https://weavatar.com/avatar", AvatarProviderHost())
	require.Equal(t, "https://weavatar.com", AvatarProviderSite())
	require.Equal(t, "WeAvatar", AvatarProviderDisplayName())

	require.Equal(t, "", avatarProviderHosts[AvatarProviderNone])
	require.Equal(t, "", avatarProviderSites[AvatarProviderNone])
}

func TestLegacyDisableGravatarMapsToProvider(t *testing.T) {
	key, value := NormalizeLegacyValue(db.SettingDisableGravatar, "1")
	require.Equal(t, KeyAvatarProvider, key)
	require.Equal(t, AvatarProviderNone, value)

	key, value = NormalizeLegacyValue(db.SettingDisableGravatar, "0")
	require.Equal(t, KeyAvatarProvider, key)
	require.Equal(t, AvatarProviderWeAvatar, value)
}
