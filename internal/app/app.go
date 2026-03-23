package app

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/policy"
	claudeprovider "github.com/canhta/vibegram/internal/providers/claude"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/runner"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

type App struct {
	cfg     config.Config
	store   *state.Store
	bot     botClient
	codex   sessionRunner
	claude  sessionRunner
	policy  policyEngine
	support supportResponder
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
		var strongCaller roles.Caller
		if strongModel := strings.TrimSpace(cfg.OpenAI.StrongModel); strongModel != "" && strongModel != cfg.OpenAI.Model {
			strongCaller = roles.NewOpenAICaller(cfg.OpenAI.APIKey, strongModel, cfg.OpenAI.BaseURL)
		}
		engine = policy.NewEngine(roles.NewExecutor(caller, strongCaller))
		support = roles.NewSupportResponder(caller, strongCaller)
	}

	return &App{
		cfg:     cfg,
		store:   state.NewStore(cfg.Runtime.StateDir),
		bot:     telegram.NewClient(cfg.Telegram.BotToken, ""),
		codex:   codexprovider.NewSessionRunner(runner.New(), resolveCommandPath(cfg.Providers.CodexCommand, "codex")),
		claude:  claudeprovider.NewSessionRunner(runner.New(), resolveCommandPath(cfg.Providers.ClaudeCommand, "claude")),
		policy:  engine,
		support: support,
	}, nil
}

func (a *App) Config() config.Config {
	return a.cfg
}

func (a *App) Run(ctx context.Context) error {
	if err := a.store.Init(); err != nil {
		return err
	}
	if err := a.registerTelegramCommands(ctx); err != nil {
		return err
	}

	runtime := NewRuntime(a.cfg, a.store, a.bot, a.codex, a.claude, a.policy, a.support)
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
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(500 * time.Millisecond):
				continue
			}
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

func (a *App) registerTelegramCommands(ctx context.Context) error {
	if a.bot == nil {
		return nil
	}

	err := a.bot.SetCommands(ctx, a.cfg.Telegram.ForumChatID, []telegram.BotCommand{
		{Command: "new", Description: "Start a new session"},
		{Command: "status", Description: "Show current status"},
		{Command: "cleanup", Description: "Delete session topics"},
	})
	if err != nil && ctx.Err() != nil {
		return nil
	}
	return err
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
