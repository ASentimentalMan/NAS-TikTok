package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ASentimentalMan/NAS-TikTok/server/internal/config"
)

type Handler struct {
	basePath string
	apiBase  string
	root     string
}

func New(cfg config.Config) *Handler {
	return &Handler{
		basePath: cfg.BasePath,
		apiBase:  cfg.PublicAPIBase,
		root:     cfg.StaticRoot,
	}
}

func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	requestPath := request.URL.Path
	if handler.basePath != "" {
		if requestPath == handler.basePath {
			http.Redirect(writer, request, handler.basePath+"/", http.StatusTemporaryRedirect)
			return
		}
		requestPath = strings.TrimPrefix(requestPath, handler.basePath)
	}
	if requestPath == "/env.js" {
		handler.serveEnvironment(writer)
		return
	}
	if requestPath == "" {
		requestPath = "/"
	}

	if requestPath == "/" {
		handler.serveFile(writer, request, "/index.html")
		return
	}
	clean := path.Clean("/" + requestPath)
	if clean != requestPath {
		http.NotFound(writer, request)
		return
	}
	if handler.fileExists(strings.TrimPrefix(clean, "/")) {
		handler.serveFile(writer, request, clean)
		return
	}
	if path.Ext(clean) == "" && strings.Contains(request.Header.Get("Accept"), "text/html") {
		handler.serveFile(writer, request, "/index.html")
		return
	}
	http.NotFound(writer, request)
}

func (handler *Handler) serveEnvironment(writer http.ResponseWriter) {
	apiBase, _ := json.Marshal(handler.apiBase)
	writer.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	_, _ = writer.Write([]byte("window.__APP_CONFIG__ = { API_BASE: " + string(apiBase) + " };\n"))
}

func (handler *Handler) fileExists(relative string) bool {
	info, err := os.Stat(filepath.Join(handler.root, filepath.FromSlash(relative)))
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func (handler *Handler) serveFile(writer http.ResponseWriter, request *http.Request, name string) {
	if name == "/index.html" {
		writer.Header().Set("Cache-Control", "no-cache")
		name = "/"
		http.ServeFile(writer, requestWithPath(request, name), handler.root)
		return
	}
	http.ServeFile(writer, requestWithPath(request, name), filepath.Join(handler.root, filepath.FromSlash(strings.TrimPrefix(name, "/"))))
}

func requestWithPath(request *http.Request, name string) *http.Request {
	copy := request.Clone(request.Context())
	copy.URL.Path = name
	copy.URL.RawPath = ""
	copy.RequestURI = name
	return copy
}
