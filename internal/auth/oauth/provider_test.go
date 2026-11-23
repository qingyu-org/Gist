package oauth

import (
	"crypto/md5"
	"fmt"
	"testing"

	"github.com/qingyu-org/Gist/internal/db"
	"github.com/stretchr/testify/require"
)

func TestSyncAvatarHashUsesAccountEmail(t *testing.T) {
	user := &db.User{Email: "Owner@Example.com"}

	syncAvatarHash(user, "provider@example.com")

	require.Equal(t, "Owner@Example.com", user.Email,
		"the provider address must not replace one the account already has")
	require.Equal(t, fmt.Sprintf("%x", md5.Sum([]byte("owner@example.com"))), user.MD5Hash,
		"the hash follows the account address, lowercased")
}

func TestSyncAvatarHashFillsMissingEmail(t *testing.T) {
	user := &db.User{}

	syncAvatarHash(user, "  New@Example.com ")

	require.Equal(t, "new@example.com", user.Email)
	require.Equal(t, fmt.Sprintf("%x", md5.Sum([]byte("new@example.com"))), user.MD5Hash)
}

func TestSyncAvatarHashWithoutAnyEmail(t *testing.T) {
	user := &db.User{MD5Hash: "stale"}

	syncAvatarHash(user, "   ")

	require.Empty(t, user.MD5Hash,
		"an account without an address has no picture hash")
}
