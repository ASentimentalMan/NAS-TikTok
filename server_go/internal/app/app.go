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

	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/auth"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/captcha"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/config"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/httpapi"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/media/catalog"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/session"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/web"
)

type App struct {
	config config.Config
	server *http.Server
	media  *catalog.Catalog
}

func New(cfg config.Config) (*App, error) {
	media, err := catalog.New(cfg.MediaRoot)
	if err != nil {
		return nil, fmt.Errorf("initialize media catalog: %w", err)
	}
	captchas, err := captcha.New(cfg.CaptchaTTL, cfg.CaptchaLimit)
	if err != nil {
		_ = media.Close()
		return nil, fmt.Errorf("initialize captcha service: %w", err)
	}
	sessions, err := session.New(cfg.JWTSecret, cfg.SessionTTL, cfg.SessionLimit)
	if err != nil {
		_ = media.Close()
		return nil, fmt.Errorf("initialize session service: %w", err)
	}
	handler := httpapi.New(cfg, auth.New(cfg.Account, cfg.Password), captchas, sessions, media)
	handler.SetWeb(web.New(cfg))
	server := &http.Server{
		Addr:              net.JoinHostPort(cfg.ListenHost, strconv.Itoa(cfg.Port)),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
	if cfg.SSL {
		server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return &App{config: cfg, server: server, media: media}, nil
}

func (application *App) Run(ctx context.Context) error {
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
	log.Printf("starting %s on %s://%s", application.config.AppName, protocol, application.server.Addr)

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
	return application.media.Close()
}
