package app

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/policy"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	opencodeprovider "github.com/canhta/vibegram/internal/providers/opencode"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/runner"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

type App struct {
	cfg      config.Config
	store    *state.Store
	bot      botClient
	codex    sessionRunner
	opencode sessionRunner
	policy   policyEngine
	support  supportResponder
}

func New(cfg config.Config) (*App, error) {
	if cfg.Telegram.BotToken == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}
	if cfg.Runtime.StateDir == "" {
		return nil, fmt.Errorf("state dir is required")
	}

	var engine policyEngine
	var support supportResponder
	if cfg.OpenAI.APIKey != "" {
		caller := roles.NewOpenAICaller(cfg.OpenAI.APIKey, cfg.OpenAI.Model, cfg.OpenAI.BaseURL)
		engine = policy.NewEngine(roles.NewExecutor(caller))
		support = roles.NewSupportResponder(caller)
	}

	return &App{
		cfg:      cfg,
		store:    state.NewStore(cfg.Runtime.StateDir),
		bot:      telegram.NewClient(cfg.Telegram.BotToken, ""),
		codex:    codexprovider.NewSessionRunner(runner.New(), resolveCommandPath(cfg.Providers.CodexCommand, "codex")),
		opencode: opencodeprovider.NewSessionRunner(runner.New(), resolveCommandPath(cfg.Providers.OpenCodeCommand, "opencode")),
		policy:   engine,
		support:  support,
	}, nil
}

func (a *App) Config() config.Config {
	return a.cfg
}

func (a *App) Run(ctx context.Context) error {
	if err := a.store.Init(); err != nil {
		return err
	}

	runtime := NewRuntime(a.cfg, a.store, a.bot, a.codex, a.opencode, a.policy, a.support)
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

func resolveCommandPath(configured, fallback string) string {
	value := configured
	if value == "" {
		value = fallback
	}
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return value
	}
	resolved, err := exec.LookPath(value)
	if err != nil {
		return value
	}
	return resolved
}
