package db

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/qingyu-org/Gist/internal/config"
)

// GistComment is a Markdown comment left on a gist. Comments are stored in the
// database only, not in the gist's git repository, and are removed along with
// their gist or their author.
//
// A comment may answer another one. Replies are one level deep: a reply to a
// reply is stored as a reply to the same top-level comment, so that a thread
// never nests further. ParentID is 0 for a top-level comment.
//
// Guests can comment when the instance allows it. Such a comment has no
// UserID and carries the name its author typed in instead.
type GistComment struct {
	ID uint `gorm:"primaryKey"`
	// GistID and CreatedAt share an index: every comment query filters on the
	// gist and orders by the creation date.
	GistID    uint   `gorm:"index:idx_gist_comments_gist_created,priority:1"`
	UserID    *uint  `gorm:"index"`
	User      *User  `validate:"-"`
	ParentID  uint   `gorm:"index"`
	GuestName string `gorm:"size:64"`
	Content   string `gorm:"type:text"`
	CreatedAt int64  `gorm:"index:idx_gist_comments_gist_created,priority:2"`
	UpdatedAt int64
}

// IsRoot reports whether the comment is a top-level one, not a reply.
func (comment *GistComment) IsRoot() bool {
	return comment.ParentID == 0
}

// IsGuest reports whether the comment was posted without signing in.
func (comment *GistComment) IsGuest() bool {
	return comment.UserID == nil
}

// AuthorName is the name to show for the comment's author: the username of a
// signed-in user, or the name a guest typed in.
func (comment *GistComment) AuthorName() string {
	if comment.User != nil {
		return comment.User.Username
	}
	return comment.GuestName
}

// WrittenByGistOwner reports whether the comment was posted by the owner of
// the gist it is on.
func (comment *GistComment) WrittenByGistOwner(gist *Gist) bool {
	return comment.UserID != nil && *comment.UserID == gist.UserID
}

// GetComments returns every comment posted on the gist, replies included,
// oldest first, with their author preloaded.
func (gist *Gist) GetComments() ([]*GistComment, error) {
	var comments []*GistComment
	err := db.Preload("User").
		Where("gist_id = ?", gist.ID).
		Order("created_at asc, id asc").
		Find(&comments).Error
	return comments, err
}

// GetCommentsPage returns a page of every comment posted on the gist, replies
// included, oldest first. It backs the API, which lists comments flat.
// `offset` is the page index (0-based); `limit` caps the returned slice (pass
// perPage+1 for the peek-next sentinel) and `perPage` is the page size used
// for the offset arithmetic.
func (gist *Gist) GetCommentsPage(offset int, limit int, perPage int) ([]*GistComment, error) {
	var comments []*GistComment
	err := db.Preload("User").
		Where("gist_id = ?", gist.ID).
		Order("created_at asc, id asc").
		Limit(limit).
		Offset(offset * perPage).
		Find(&comments).Error
	return comments, err
}

// GetRootCommentsPage returns a page of the gist's top-level comments, oldest
// first. `offset` is the page index (0-based); `limit` caps the returned slice
// (pass perPage+1 for the peek-next sentinel) and `perPage` is the page size
// used for the offset arithmetic. Replies are loaded separately with
// GetReplies, so that a page never starts with an orphaned reply.
func (gist *Gist) GetRootCommentsPage(offset int, limit int, perPage int) ([]*GistComment, error) {
	var comments []*GistComment
	err := db.Preload("User").
		Where("gist_id = ? AND parent_id = 0", gist.ID).
		Order("created_at asc, id asc").
		Limit(limit).
		Offset(offset * perPage).
		Find(&comments).Error
	return comments, err
}

// GetLastRootComments returns the last n top-level comments of the gist,
// oldest first, with their author preloaded.
func (gist *Gist) GetLastRootComments(n int) ([]*GistComment, error) {
	var comments []*GistComment
	err := db.Preload("User").
		Where("gist_id = ? AND parent_id = 0", gist.ID).
		Order("created_at desc, id desc").
		Limit(n).
		Find(&comments).Error
	slices.Reverse(comments)
	return comments, err
}

// GetReplies returns every reply to the given top-level comments, oldest
// first, with their author preloaded.
func (gist *Gist) GetReplies(rootIDs []uint) ([]*GistComment, error) {
	replies := make([]*GistComment, 0)
	if len(rootIDs) == 0 {
		return replies, nil
	}
	err := db.Preload("User").
		Where("gist_id = ? AND parent_id IN ?", gist.ID, rootIDs).
		Order("created_at asc, id asc").
		Find(&replies).Error
	return replies, err
}

// CountComments counts every comment of the gist, replies included.
func (gist *Gist) CountComments() (int64, error) {
	var count int64
	err := db.Model(&GistComment{}).Where("gist_id = ?", gist.ID).Count(&count).Error
	return count, err
}

// CountRootComments counts the gist's top-level comments. It is what the
// comment pages are numbered by, each page holding whole threads.
func (gist *Gist) CountRootComments() (int64, error) {
	var count int64
	err := db.Model(&GistComment{}).Where("gist_id = ? AND parent_id = 0", gist.ID).Count(&count).Error
	return count, err
}

// RootPosition returns how many top-level comments were posted on the gist
// before this one's thread, in the same oldest first order as
// GetRootCommentsPage. It must be called on a top-level comment.
func (comment *GistComment) RootPosition() (int64, error) {
	var count int64
	err := db.Model(&GistComment{}).
		Where("gist_id = ? AND parent_id = 0 AND (created_at < ? OR (created_at = ? AND id < ?))",
			comment.GistID, comment.CreatedAt, comment.CreatedAt, comment.ID).
		Count(&count).Error
	return count, err
}

// GetGistComment returns the comment commentID if it belongs to the gist
// gistID, with its author preloaded.
func GetGistComment(gistID uint, commentID uint) (*GistComment, error) {
	comment := new(GistComment)
	err := db.Preload("User").
		Where("id = ? AND gist_id = ?", commentID, gistID).
		First(&comment).Error
	return comment, err
}

// Create inserts the comment and increments the gist's comment counter.
func (comment *GistComment) Create() error {
	// Same instant for both, so a fresh comment is never reported as edited
	now := time.Now().Unix()
	comment.CreatedAt = now
	comment.UpdatedAt = now

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit(clause.Associations).Create(comment).Error; err != nil {
			return err
		}
		return tx.Model(&Gist{}).
			Where("id = ?", comment.GistID).
			UpdateColumn("nb_comments", gorm.Expr("nb_comments + 1")).Error
	})
}

// Update saves the comment's content, bumping its UpdatedAt.
func (comment *GistComment) Update() error {
	return db.Model(comment).Update("content", comment.Content).Error
}

// Delete removes the comment along with its replies, and decrements the gist's
// comment counter by as many comments as were removed.
func (comment *GistComment) Delete() error {
	return db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("id = ? OR parent_id = ?", comment.ID, comment.ID).Delete(&GistComment{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Already deleted by a concurrent request, which decremented the counter
			return nil
		}
		return tx.Model(&Gist{}).
			Where("id = ?", comment.GistID).
			UpdateColumn("nb_comments", gorm.Expr("nb_comments - ?", res.RowsAffected)).Error
	})
}

// NotifyParticipants puts the comment in the inbox of whoever should read it:
// the author of the comment it replies to, or the owner of the gist for a
// top-level comment. A guest has no inbox, so a reply to a guest comment goes
// to the gist owner. Nobody is notified of their own comment.
//
// The caller logs the error rather than failing the comment: the author
// already sees it posted.
func (comment *GistComment) NotifyParticipants(gist *Gist) error {
	// The gist is always loaded with its owner, whose name the link needs
	if !config.C.CommentNotify || gist.User.ID == 0 {
		return nil
	}

	recipient := gist.UserID
	kind := "comment"
	titleKey := "notification.comment.title"
	bodyKey := "notification.comment.body"

	if !comment.IsRoot() {
		parent, err := GetGistComment(gist.ID, comment.ParentID)
		if err == nil && parent.UserID != nil {
			recipient = *parent.UserID
			kind = "reply"
			titleKey = "notification.reply.title"
			bodyKey = "notification.reply.body"
		}
	}

	// Includes the case of a guest replying to themselves under the same name
	if comment.UserID != nil && *comment.UserID == recipient {
		return nil
	}

	title := gist.Title
	if title == "" {
		title = gist.Identifier()
	}
	link := "/" + gist.User.Username + "/" + gist.Identifier() +
		"#comment-" + strconv.FormatUint(uint64(comment.ID), 10)

	return CreateNotification(kind, recipient, titleKey, bodyKey,
		[]string{comment.AuthorName(), title}, link)
}

// IsEdited reports whether the comment content was changed after posting.
func (comment *GistComment) IsEdited() bool {
	return comment.UpdatedAt > comment.CreatedAt
}

// CanEdit reports whether user may change the comment: only its author can.
// Guest comments have no author to prove themselves, so nobody can edit them.
func (comment *GistComment) CanEdit(user *User) bool {
	return user != nil && comment.UserID != nil && *comment.UserID == user.ID
}

// CanDelete reports whether user may delete the comment: its author, the
// owner of the gist it was posted on, or an admin.
func (comment *GistComment) CanDelete(user *User, gist *Gist) bool {
	if user == nil {
		return false
	}
	return (comment.UserID != nil && *comment.UserID == user.ID) ||
		gist.UserID == user.ID || user.IsAdmin
}

// -- DTO -- //

type GistCommentDTO struct {
	Content   string `validate:"required,max=3000" form:"content"`
	ParentID  uint   `form:"parent_id"`
	GuestName string `validate:"max=32" form:"name"`
}

// Normalize converts CRLF line endings (sent by browser textareas) to LF and
// trims leading blank lines and trailing whitespace, which don't change the
// rendered Markdown. Content made only of whitespace ends up empty, so that it
// fails the required rule. The guest name is trimmed too, so that a name made
// only of spaces is treated as missing.
func (dto *GistCommentDTO) Normalize() {
	content := strings.ReplaceAll(dto.Content, "\r\n", "\n")
	content = strings.TrimRight(content, " \t\r\n")
	content = strings.TrimLeft(content, "\n")
	if strings.TrimSpace(content) == "" {
		content = ""
	}
	dto.Content = content
	dto.GuestName = strings.TrimSpace(dto.GuestName)
}
