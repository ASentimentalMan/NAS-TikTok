package streaming

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ASentimentalMan/NAS-TikTok/server/internal/media/catalog"
)

type FileOpener interface {
	Open(string) (*os.File, fs.FileInfo, error)
}

type Handler struct {
	files         FileOpener
	removeMissing func(string) error
}

func New(files FileOpener, removeMissing func(string) error) *Handler {
	return &Handler{files: files, removeMissing: removeMissing}
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request, relative string) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	file, info, err := handler.files.Open(relative)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if handler.removeMissing != nil {
				_ = handler.removeMissing(relative)
			}
			http.Error(writer, "file not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, catalog.ErrUnsafePath) || errors.Is(err, catalog.ErrUnsupported) {
			http.Error(writer, "file not found", http.StatusNotFound)
			return
		}
		http.Error(writer, "file unavailable", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	contentType := contentType(filepath.Ext(relative))
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Accept-Ranges", "bytes")
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if disposition := mime.FormatMediaType("inline", map[string]string{"filename": filepath.Base(relative)}); disposition != "" {
		writer.Header().Set("Content-Disposition", disposition)
	}
	http.ServeContent(writer, request, filepath.Base(relative), info.ModTime(), file)
}

func contentType(extension string) string {
	switch strings.ToLower(extension) {
	case ".jpg", ".jpeg", ".jfif":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".avif":
		return "image/avif"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	case ".gif":
		return "image/gif"
	case ".apng":
		return "image/apng"
	case ".bmp":
		return "image/bmp"
	case ".jxl":
		return "image/jxl"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	default:
		return "application/octet-stream"
	}
}
