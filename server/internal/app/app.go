package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/ASentimentalMan/NAS-TikTok/server/internal/auth"
	"github.com/ASentimentalMan/NAS-TikTok/server/internal/captcha"
	"github.com/ASentimentalMan/NAS-TikTok/server/internal/config"
	"github.com/ASentimentalMan/NAS-TikTok/server/internal/httpapi"
	"github.com/ASentimentalMan/NAS-TikTok/server/internal/logging"
	"github.com/ASentimentalMan/NAS-TikTok/server/internal/media/catalog"
	"github.com/ASentimentalMan/NAS-TikTok/server/internal/media/index"
	"github.com/ASentimentalMan/NAS-TikTok/server/internal/session"
	"github.com/ASentimentalMan/NAS-TikTok/server/internal/web"
)

type App struct {
	config config.Config
	server *http.Server
	media  *catalog.Catalog
	index  *index.Manager
	logger *logging.Logger
}

func New(cfg config.Config) (*App, error) {
	logger := logging.New(cfg.LogLevel, log.Default())
	media, err := catalog.New(cfg.MediaRoot)
	if err != nil {
		return nil, fmt.Errorf("initialize media catalog: %w", err)
	}
	mediaIndex, err := index.New(cfg.DatabasePath, cfg.MediaRoot, catalogScanner{catalog: media}, logger)
	if err != nil {
		logger.Errorf("event=media_index_init_failed error=%q", err)
		mediaIndex = nil
	}
	captchas, err := captcha.New(cfg.CaptchaTTL, cfg.CaptchaLimit)
	if err != nil {
		if mediaIndex != nil {
			_ = mediaIndex.Close()
		}
		_ = media.Close()
		return nil, fmt.Errorf("initialize captcha service: %w", err)
	}
	sessions, err := session.New(cfg.JWTSecret, cfg.SessionTTL, cfg.SessionLimit)
	if err != nil {
		if mediaIndex != nil {
			_ = mediaIndex.Close()
		}
		_ = media.Close()
		return nil, fmt.Errorf("initialize session service: %w", err)
	}
	authenticator := auth.New(cfg.Account, cfg.Password)
	var handler *httpapi.Handler
	if mediaIndex == nil {
		handler = httpapi.New(cfg, authenticator, captchas, sessions, media, nil, logger)
	} else {
		handler = httpapi.New(cfg, authenticator, captchas, sessions, media, mediaIndex, logger)
	}
	handler.SetWeb(web.New(cfg))
	server := &http.Server{
		Addr:              net.JoinHostPort(cfg.ListenHost, strconv.Itoa(cfg.Port)),
		Handler:           handler,
		ErrorLog:          logger.ErrorLogger(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
	if cfg.SSL {
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return &App{config: cfg, server: server, media: media, index: mediaIndex, logger: logger}, nil
}

func (application *App) Run(ctx context.Context) error {
	if application.index != nil {
		if err := application.index.Start(ctx); err != nil {
			application.logger.Errorf("event=media_index_start_failed error=%q", err)
		}
	}
	result := make(chan error, 1)
	go func() {
		if application.config.SSL {
			result <- application.server.ListenAndServeTLS(application.config.TLSCertFile, application.config.TLSKeyFile)
			return
		}
		result <- application.server.ListenAndServe()
	}()
	protocol := "http"
	if application.config.SSL {
		protocol = "https"
	}
	application.logger.Debugf("event=server_start protocol=%s address=%s", protocol, application.server.Addr)

	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server stopped: %w", err)
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := application.server.Shutdown(shutdown); err != nil {
			_ = application.server.Close()
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}
		if err := <-result; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server stopped: %w", err)
		}
		return nil
	}
}

func (application *App) Close() error {
	var indexErr error
	if application.index != nil {
		indexErr = application.index.Close()
	}
	return errors.Join(indexErr, application.media.Close())
}

type catalogScanner struct {
	catalog *catalog.Catalog
}

func (scanner catalogScanner) ScanDirectory(ctx context.Context, relativeDir string) ([]index.MediaFile, []string, error) {
	files, directories, err := scanner.catalog.ScanDirectory(ctx, relativeDir)
	if err != nil {
		return nil, nil, err
	}
	indexedFiles := make([]index.MediaFile, len(files))
	for position, file := range files {
		indexedFiles[position] = index.MediaFile{RelativePath: file.RelativePath, MediaType: file.MediaType}
	}
	return indexedFiles, directories, nil
}
