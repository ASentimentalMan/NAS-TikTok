package streaming

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/media/catalog"
)

type FileOpener interface {
	Open(string) (*os.File, fs.FileInfo, error)
}

type Handler struct {
	files FileOpener
}

func New(files FileOpener) *Handler {
	return &Handler{files: files}
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request, relative string) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	file, info, err := handler.files.Open(relative)
	if err != nil {
		if errors.Is(err, catalog.ErrUnsafePath) || errors.Is(err, catalog.ErrUnsupported) || errors.Is(err, os.ErrNotExist) {
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
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	case ".avif":
		return "image/avif"
	case ".webp":
		return "image/webp"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".apng":
		return "image/apng"
	case ".bmp":
		return "image/bmp"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".ogv", ".ogg":
		return "video/ogg"
	default:
		if value := mime.TypeByExtension(extension); value != "" {
			return value
		}
		return "application/octet-stream"
	}
}
