package gist

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/qingyu-org/Gist/internal/config"
	"github.com/qingyu-org/Gist/internal/settings"
	"github.com/qingyu-org/Gist/internal/web/context"
)

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// mediaExtensions is the set of extensions treated as media when media uploads
// are disabled. Everything else is still allowed as a regular attachment.
var mediaExtensions = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {}, ".avif": {},
	".svg": {}, ".bmp": {}, ".ico": {}, ".tiff": {}, ".heic": {},
	".mp3": {}, ".wav": {}, ".ogg": {}, ".flac": {}, ".m4a": {},
	".mp4": {}, ".webm": {}, ".mov": {}, ".mkv": {}, ".avi": {},
}

func extensionOf(name string) string {
	return strings.ToLower(filepath.Ext(name))
}

func isBlockedExtension(name string) bool {
	ext := strings.TrimPrefix(extensionOf(name), ".")
	if ext == "" {
		return false
	}
	for _, blocked := range settings.BlockedExtensions() {
		blocked = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(blocked, ".")))
		if blocked != "" && blocked == ext {
			return true
		}
	}
	return false
}

func isMediaFile(name string) bool {
	_, ok := mediaExtensions[extensionOf(name)]
	return ok
}

func Upload(ctx *context.Context) error {
	if !settings.UploadEnabled() {
		return ctx.ErrorRes(http.StatusForbidden, ctx.Tr("error.file-upload-disabled"), nil)
	}

	maxFileBytes := settings.UploadMaxFileBytes()
	// ParseMultipartForm also buffers small fields in memory; give the body a
	// little headroom over the single-file limit so form fields still fit.
	ctx.Request().Body = http.MaxBytesReader(nil, ctx.Request().Body, maxFileBytes+1<<20)

	if err := ctx.Request().ParseMultipartForm(8 << 20); err != nil {
		return ctx.ErrorRes(http.StatusBadRequest, ctx.Tr("error.file-too-large"), err)
	}

	fileHeader, err := ctx.FormFile("file")
	if err != nil {
		return ctx.ErrorRes(http.StatusBadRequest, ctx.Tr("error.no-file-uploaded"), err)
	}

	if fileHeader.Size > maxFileBytes {
		return ctx.ErrorRes(http.StatusRequestEntityTooLarge, ctx.Tr("error.file-too-large"), nil)
	}

	if isBlockedExtension(fileHeader.Filename) {
		return ctx.ErrorRes(http.StatusBadRequest, ctx.Tr("error.file-type-blocked"), nil)
	}

	if !settings.UploadMediaAllowed() && isMediaFile(fileHeader.Filename) {
		return ctx.ErrorRes(http.StatusBadRequest, ctx.Tr("error.media-disabled"), nil)
	}

	file, err := fileHeader.Open()
	if err != nil {
		return ctx.ErrorRes(http.StatusBadRequest, ctx.Tr("error.cannot-open-file"), err)
	}
	defer file.Close()

	fileUUID, err := uuid.NewRandom()
	if err != nil {
		return ctx.ErrorRes(http.StatusInternalServerError, "Error generating UUID", err)
	}

	uploadsDir := filepath.Join(config.GetHomeDir(), "uploads")
	if err := os.MkdirAll(uploadsDir, 0755); err != nil {
		return ctx.ErrorRes(http.StatusInternalServerError, "Error creating uploads directory", err)
	}

	filename := fileUUID.String()
	filePath := filepath.Join(uploadsDir, filename)

	destFile, err := os.Create(filePath)
	if err != nil {
		return ctx.ErrorRes(http.StatusInternalServerError, "Error creating file", err)
	}
	defer destFile.Close()

	written, err := io.Copy(destFile, io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		_ = os.Remove(filePath)
		return ctx.ErrorRes(http.StatusInternalServerError, "Error saving file", err)
	}
	if written > maxFileBytes {
		_ = os.Remove(filePath)
		return ctx.ErrorRes(http.StatusRequestEntityTooLarge, ctx.Tr("error.file-too-large"), nil)
	}

	return ctx.JSON(http.StatusOK, map[string]string{
		"uuid":     filename,
		"filename": fileHeader.Filename,
	})
}

func DeleteUpload(ctx *context.Context) error {
	fileUuid := filepath.Base(ctx.Param("uuid"))

	if fileUuid == "" || !uuidRegex.MatchString(fileUuid) {
		return ctx.ErrorRes(http.StatusBadRequest, ctx.Tr("error.bad-request"), nil)
	}

	filePath := filepath.Join(config.GetHomeDir(), "uploads", fileUuid)

	if _, err := os.Stat(filePath); err == nil {
		if err := os.Remove(filePath); err != nil {
			return ctx.ErrorRes(http.StatusInternalServerError, "Error deleting file", err)
		}
	}

	return ctx.JSON(http.StatusOK, map[string]string{
		"status": "deleted",
	})
}
