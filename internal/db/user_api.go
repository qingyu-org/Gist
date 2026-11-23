package db

import (
	"time"

	"github.com/qingyu-org/Gist/internal/web/handlers/api/v1/types"
)

// AvatarURLBuilder turns a Gravatar hash into the picture URL the site uses.
// The settings package installs it at startup: this keeps the API reporting the
// same picture the pages draw, without db importing settings (settings imports
// db, so the other direction would be a cycle). Empty hash means "no picture".
var AvatarURLBuilder = func(hash string) string { return "" }

// ToSimpleAPI returns the public simple-user shape used inside gist responses
// and by the public user-lookup endpoints. It carries no private fields (no
// email). Fields whose underlying feature doesn't exist in QingyuGist
// (followers, repos, ...) are still populated with the spec-shaped URLs so
// clients can parse cleanly.
func (u *User) ToSimpleAPI() types.SimpleUser {
	return types.SimpleUser{
		ID:        u.ID,
		Login:     u.Username,
		Username:  u.Username,
		AvatarURL: AvatarURLBuilder(u.MD5Hash),
		Type:      "User",
		CreatedAt: time.Unix(u.CreatedAt, 0).UTC(),
	}
}

// ToPrivateAPI returns the self shape for the authenticated-user endpoints
// (GET/PATCH /user): the public fields plus the caller's own email.
func (u *User) ToPrivateAPI() types.PrivateUser {
	return types.PrivateUser{
		SimpleUser: u.ToSimpleAPI(),
		Email:      u.Email,
	}
}
