package db

import (
	"io"
	"testing"
	"time"

	"github.com/qingyu-org/Gist/internal/config"
	"github.com/stretchr/testify/require"
)

// TestNotExpired - the filter applied to gist listings keeps the gists that
// never expire, including those created before expiration existed, whose
// expires_at is NULL.
func TestNotExpired(t *testing.T) {
	require.NoError(t, config.InitConfig(io.Discard))
	config.C.QingyuGistHome = t.TempDir()
	require.NoError(t, Setup("opengist.db"))
	t.Cleanup(func() { _ = Close() })

	now := time.Now().Unix()
	for uuid, expiresAt := range map[string]any{
		"legacy":  nil,
		"never":   0,
		"future":  now + 3600,
		"expired": now - 60,
	} {
		require.NoError(t, db.Exec("INSERT INTO gists (uuid, expires_at) VALUES (?, ?)", uuid, expiresAt).Error)
	}

	var uuids []string
	require.NoError(t, db.Model(&Gist{}).Scopes(notExpired).Pluck("uuid", &uuids).Error)
	require.ElementsMatch(t, []string{"legacy", "never", "future"}, uuids)
}
