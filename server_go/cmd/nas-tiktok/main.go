package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/app"
	"github.com/ASentimentalMan/NAS-TikTok/server_go/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("configuration error: %v", err)
		os.Exit(1)
	}
	application, err := app.New(cfg)
	if err != nil {
		log.Printf("startup error: %v", err)
		os.Exit(1)
	}
	defer application.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := application.Run(ctx); err != nil {
		log.Printf("server error: %v", err)
		os.Exit(1)
	}
}
