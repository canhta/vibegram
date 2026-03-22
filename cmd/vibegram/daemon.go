package main

import (
	"context"
	"log"

	"github.com/canhta/vibegram/internal/app"
	"github.com/canhta/vibegram/internal/config"
)

func runDaemon(ctx context.Context, loadConfig func() (config.Config, error)) error {
	cfg, err := loadConfig()
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
