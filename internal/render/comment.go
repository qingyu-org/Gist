package render

import (
	"html/template"

	"github.com/qingyu-org/Gist/internal/db"
	"github.com/rs/zerolog/log"
)

// RenderedComment is a gist comment with its Markdown content rendered to HTML
// and, for a top-level comment, the replies it received.
type RenderedComment struct {
	*db.GistComment
	HTML    string
	Replies []*RenderedComment
}

// RenderComments renders the Markdown content of the comments, with the same
// renderer as Markdown files and previews. A comment that fails to render is
// shown as preformatted text instead.
func RenderComments(comments []*db.GistComment) []*RenderedComment {
	rendered := make([]*RenderedComment, 0, len(comments))
	for _, comment := range comments {
		html, err := MarkdownString(comment.Content)
		if err != nil {
			log.Error().Err(err).Msgf("Error rendering comment %d", comment.ID)
			html = "<pre>" + template.HTMLEscapeString(comment.Content) + "</pre>"
		}
		rendered = append(rendered, &RenderedComment{GistComment: comment, HTML: html})
	}
	return rendered
}

// RenderCommentThreads renders the comments and hangs every reply under the
// top-level comment it answers, so the returned slice holds whole threads,
// oldest first. A reply whose parent is not part of the slice is shown as a
// top-level comment rather than dropped.
func RenderCommentThreads(comments []*db.GistComment) []*RenderedComment {
	rendered := RenderComments(comments)

	byID := make(map[uint]*RenderedComment, len(rendered))
	for _, comment := range rendered {
		byID[comment.ID] = comment
	}

	threads := make([]*RenderedComment, 0, len(rendered))
	for _, comment := range rendered {
		if comment.IsRoot() {
			threads = append(threads, comment)
			continue
		}
		if parent, ok := byID[comment.ParentID]; ok {
			parent.Replies = append(parent.Replies, comment)
			continue
		}
		threads = append(threads, comment)
	}
	return threads
}
