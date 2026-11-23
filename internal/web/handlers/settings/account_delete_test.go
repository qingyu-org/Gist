package settings_test

import (
	"testing"
	"time"

	"github.com/qingyu-org/Gist/internal/db"
	webtest "github.com/qingyu-org/Gist/internal/web/test"
	"github.com/stretchr/testify/require"
)

// Deleting an account has to take every row owned by it with it. The second
// factor used to be left behind, which made the delete fail outright on
// databases that enforce foreign keys (SQLite does), and notifications and the
// activity calendar were never cleaned up.
func TestDeleteAccountRemovesOwnedRows(t *testing.T) {
	s := webtest.Setup(t)
	defer webtest.Teardown(t)

	s.Register(t, "thomas")
	s.Login(t, "thomas")
	user, err := db.GetUserByUsername("thomas")
	require.NoError(t, err)

	totp := &db.TOTP{UserID: user.ID}
	require.NoError(t, totp.StoreSecret("JBSWY3DPEHPK3PXP"))
	require.NoError(t, totp.Create())

	require.NoError(t, db.CreateNotification("system", user.ID, "notification.title", "notification.body", nil, "/"))
	require.NoError(t, db.SetActivityDay(user.ID, time.Now().Format("2006-01-02"), 3))

	unread, err := db.CountUnreadNotifications(user.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), unread)

	s.Request(t, "DELETE", "/settings/account", nil, 302)

	_, err = db.GetUserByUsername("thomas")
	require.Error(t, err, "the account should be gone")

	_, err = db.GetTOTPByUserID(user.ID)
	require.Error(t, err, "the second factor should be gone")

	unread, err = db.CountUnreadNotifications(user.ID)
	require.NoError(t, err)
	require.Zero(t, unread, "the account's notifications should be gone")
}
