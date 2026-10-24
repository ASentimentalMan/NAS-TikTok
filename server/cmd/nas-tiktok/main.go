package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ASentimentalMan/NAS-TikTok/server/internal/app"
	"github.com/ASentimentalMan/NAS-TikTok/server/internal/config"
	"github.com/ASentimentalMan/NAS-TikTok/server/internal/logging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		logging.FromEnv().Errorf("configuration error: %v", err)
		os.Exit(1)
	}
	logger := logging.New(cfg.LogLevel, log.Default())
	application, err := app.New(cfg)
	if err != nil {
		logger.Errorf("startup error: %v", err)
		os.Exit(1)
	}
	defer func() {
		if err := application.Close(); err != nil {
			logger.Errorf("application shutdown failed: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := application.Run(ctx); err != nil {
		logger.Errorf("server error: %v", err)
		os.Exit(1)
	}
}
