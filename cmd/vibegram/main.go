package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/canhta/vibegram/internal/app"
	"github.com/canhta/vibegram/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	application, err := app.New(cfg)
	if err != nil {
		return err
	}

	log.Printf("vibegram bootstrap ready: state_dir=%s chat_id=%d", cfg.Runtime.StateDir, cfg.Telegram.ForumChatID)
	return application.Run(ctx)
}
