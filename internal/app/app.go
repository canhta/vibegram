package app

import (
	"context"
	"fmt"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/state"
)

type App struct {
	cfg   config.Config
	store *state.Store
}

func New(cfg config.Config) (*App, error) {
	if cfg.Telegram.BotToken == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}
	if cfg.Runtime.StateDir == "" {
		return nil, fmt.Errorf("state dir is required")
	}

	return &App{
		cfg:   cfg,
		store: state.NewStore(cfg.Runtime.StateDir),
	}, nil
}

func (a *App) Config() config.Config {
	return a.cfg
}

func (a *App) Run(ctx context.Context) error {
	if err := a.store.Init(); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}
