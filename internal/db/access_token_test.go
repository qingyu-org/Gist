package db_test

import (
	"strings"
	"testing"

	"github.com/qingyu-org/Gist/internal/db"
	"github.com/stretchr/testify/require"
)

func TestGenerateToken(t *testing.T) {
	tok := &db.AccessToken{}
	plain, err := tok.GenerateToken()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(plain, db.TokenPrefix))
	require.Len(t, plain, len(db.TokenPrefix)+64)
	require.True(t, db.IsAccessToken(plain))
	require.NotEmpty(t, tok.TokenHash)
}

func TestIsAccessToken(t *testing.T) {
	require.True(t, db.IsAccessToken("qyg_abc"))
	require.True(t, db.IsAccessToken("og_abc"), "legacy tokens stay accepted")
	require.False(t, db.IsAccessToken("abc"))
	require.False(t, db.IsAccessToken(""))
}

func TestAccessTokenScopeUser(t *testing.T) {
	tok := &db.AccessToken{ScopeUser: db.ReadPermission}
	require.True(t, tok.HasUserReadPermission(), "ScopeUser=1 should grant user read")

	tokNone := &db.AccessToken{ScopeUser: 0}
	require.False(t, tokNone.HasUserReadPermission(), "ScopeUser=0 should not grant user read")
}

func TestAccessTokenDTOScopeUser(t *testing.T) {
	dto := &db.AccessTokenDTO{Name: "t", ScopeGist: 1, ScopeUser: 1}
	tok := dto.ToAccessToken()
	require.Equal(t, uint(1), tok.ScopeUser)
	require.Equal(t, uint(1), tok.ScopeGist)
}
