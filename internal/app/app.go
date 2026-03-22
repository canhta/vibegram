package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/policy"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/runner"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

type App struct {
	cfg    config.Config
	store  *state.Store
	bot    botClient
	codex  codexSessionRunner
	policy policyEngine
}

func New(cfg config.Config) (*App, error) {
	if cfg.Telegram.BotToken == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}
	if cfg.Runtime.StateDir == "" {
		return nil, fmt.Errorf("state dir is required")
	}

	var engine policyEngine
	if cfg.OpenAI.APIKey != "" {
		caller := roles.NewOpenAICaller(cfg.OpenAI.APIKey, cfg.OpenAI.Model, cfg.OpenAI.BaseURL)
		engine = policy.NewEngine(roles.NewExecutor(caller))
	}

	return &App{
		cfg:    cfg,
		store:  state.NewStore(cfg.Runtime.StateDir),
		bot:    telegram.NewClient(cfg.Telegram.BotToken, ""),
		codex:  codexprovider.NewSessionRunner(runner.New(), cfg.Providers.CodexCommand, cfg.Runtime.WorkRoot),
		policy: engine,
	}, nil
}

func (a *App) Config() config.Config {
	return a.cfg
}

func (a *App) Run(ctx context.Context) error {
	if err := a.store.Init(); err != nil {
		return err
	}

	runtime := NewRuntime(a.cfg, a.store, a.bot, a.codex, a.policy)
	offset, err := a.store.LoadCursor("telegram_updates")
	if err != nil {
		if !errors.Is(err, state.ErrNotFound) {
			return err
		}
		offset = 0
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := a.bot.GetUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		if len(updates) == 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(250 * time.Millisecond):
				continue
			}
		}

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if err := runtime.HandleUpdate(ctx, update); err != nil {
				return err
			}
			if err := a.store.SaveCursor("telegram_updates", offset); err != nil {
				return err
			}
		}
	}
}
