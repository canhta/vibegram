package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

const browseButtonLimit = 6

type draftStep string

const (
	draftStepProvider draftStep = "provider"
	draftStepStart    draftStep = "start"
	draftStepBrowse   draftStep = "browse"
	draftStepTask     draftStep = "task"
)

type browseEntry struct {
	Name string
	Path string
}

type startChoice struct {
	Label    string
	Path     string
	RootPath string
}

type generalDraft struct {
	UserID       int64
	Provider     string
	Task         string
	WorkRoot     string
	BrowseRoot   string
	CurrentPath  string
	StartChoices []startChoice
	Step         draftStep
}

func (r *Runtime) handleCallback(ctx context.Context, callback telegram.CallbackQuery) error {
	if !r.auth.CanSendCommand(callback.FromUserID) {
		return nil
	}

	threadID := callback.Message.ThreadID
	if threadID == 0 {
		threadID = 1
	}
	if threadID != 1 {
		return r.bot.AnswerCallback(ctx, callback.ID, "")
	}

	draft, ok := r.draftForUser(callback.FromUserID)
	if !ok {
		return r.bot.AnswerCallback(ctx, callback.ID, "No draft in progress")
	}

	if err := r.bot.AnswerCallback(ctx, callback.ID, ""); err != nil {
		return err
	}

	return r.handleGeneralDraftCallback(ctx, callback.Message.ChatID, draft, callback.Data)
}

func (r *Runtime) startGeneralDraft(ctx context.Context, chatID, userID int64) error {
	draft := generalDraft{
		UserID: userID,
		Step:   draftStepProvider,
	}
	r.saveDraft(draft)

	text, markup := renderProviderPrompt(r.cfg)
	_, err := r.bot.SendMessageCard(ctx, chatID, nil, text, markup)
	return err
}

func (r *Runtime) handleGeneralDraftCallback(ctx context.Context, chatID int64, draft generalDraft, data string) error {
	switch {
	case data == "wiz:cancel":
		r.clearDraft(draft.UserID)
		return r.bot.SendMessage(ctx, chatID, nil, "Cancelled. No topic created.")

	case strings.HasPrefix(data, "wiz:provider:"):
		provider := strings.TrimPrefix(data, "wiz:provider:")
		if r.runnerForProvider(provider) == nil {
			return r.bot.SendMessage(ctx, chatID, nil, "That agent is not available right now.")
		}

		choices, err := r.buildStartChoices(draft.UserID)
		if err != nil {
			return r.bot.SendMessage(ctx, chatID, nil, "I couldn't prepare folder suggestions right now.")
		}

		draft.Provider = provider
		draft.StartChoices = choices
		draft.Step = draftStepStart
		r.saveDraft(draft)

		text, markup := renderStartPrompt(draft)
		_, err = r.bot.SendMessageCard(ctx, chatID, nil, text, markup)
		return err

	case data == "wiz:start:more":
		home, err := ensureDirectory(userHomeDir())
		if err != nil {
			return r.bot.SendMessage(ctx, chatID, nil, "Home folder is not available right now.")
		}

		draft.BrowseRoot = home
		draft.CurrentPath = home
		draft.WorkRoot = ""
		draft.Step = draftStepBrowse
		r.saveDraft(draft)

		return r.sendBrowsePrompt(ctx, chatID, draft)

	case strings.HasPrefix(data, "wiz:start:"):
		index, err := strconv.Atoi(strings.TrimPrefix(data, "wiz:start:"))
		if err != nil || index < 0 || index >= len(draft.StartChoices) {
			return r.bot.SendMessage(ctx, chatID, nil, "That folder choice expired. Start again with /new.")
		}

		choice := draft.StartChoices[index]
		draft.BrowseRoot = choice.RootPath
		draft.CurrentPath = choice.Path
		draft.WorkRoot = ""
		draft.Step = draftStepBrowse
		r.saveDraft(draft)

		return r.sendBrowsePrompt(ctx, chatID, draft)

	case data == "wiz:browse:choose":
		workRoot, err := ensureDirectory(draft.CurrentPath)
		if err != nil {
			return r.bot.SendMessage(ctx, chatID, nil, "That folder is not available anymore.")
		}

		draft.WorkRoot = workRoot
		draft.Step = draftStepTask
		r.saveDraft(draft)

		return r.sendTaskPrompt(ctx, chatID, draft)

	case data == "wiz:browse:up":
		if draft.BrowseRoot == "" || draft.CurrentPath == "" {
			return r.bot.SendMessage(ctx, chatID, nil, "Choose a starting place first.")
		}

		draft.CurrentPath = browseParentPath(draft.BrowseRoot, draft.CurrentPath)
		r.saveDraft(draft)
		return r.sendBrowsePrompt(ctx, chatID, draft)

	case strings.HasPrefix(data, "wiz:browse:open:"):
		index, err := strconv.Atoi(strings.TrimPrefix(data, "wiz:browse:open:"))
		if err != nil {
			return r.bot.SendMessage(ctx, chatID, nil, "That folder choice expired. Try again.")
		}

		entries, err := browseChildren(draft.CurrentPath)
		if err != nil {
			return r.bot.SendMessage(ctx, chatID, nil, "I couldn't read that folder right now.")
		}
		if index < 0 || index >= len(entries) {
			return r.bot.SendMessage(ctx, chatID, nil, "That folder choice expired. Try again.")
		}

		draft.CurrentPath = entries[index].Path
		draft.WorkRoot = ""
		r.saveDraft(draft)
		return r.sendBrowsePrompt(ctx, chatID, draft)

	default:
		return nil
	}
}

func (r *Runtime) handleGeneralDraftText(ctx context.Context, chatID int64, draft generalDraft, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	switch draft.Step {
	case draftStepBrowse:
		return r.handleBrowseText(ctx, chatID, draft, text)

	case draftStepTask:
		draft.Task = text
		r.saveDraft(draft)
		if err := r.launchDraftSession(ctx, chatID, draft.UserID, draft); err != nil {
			return r.bot.SendMessage(ctx, chatID, nil, "launch failed: "+err.Error())
		}
		r.clearDraft(draft.UserID)
		return nil

	default:
		return r.bot.SendMessage(ctx, chatID, nil, "Use the current buttons to keep setting up this draft, or /new to restart.")
	}
}

func (r *Runtime) handleBrowseText(ctx context.Context, chatID int64, draft generalDraft, text string) error {
	switch text {
	case ".":
		draft.WorkRoot = draft.CurrentPath
		draft.Step = draftStepTask
		r.saveDraft(draft)
		return r.sendTaskPrompt(ctx, chatID, draft)
	case "..":
		draft.CurrentPath = browseParentPath(draft.BrowseRoot, draft.CurrentPath)
		r.saveDraft(draft)
		return r.sendBrowsePrompt(ctx, chatID, draft)
	}

	if strings.Contains(text, "/") || strings.Contains(text, string(filepath.Separator)+string(filepath.Separator)) {
		_ = r.bot.SendMessage(ctx, chatID, nil, "Send one folder name at a time, or use .. to go up.")
		return r.sendBrowsePrompt(ctx, chatID, draft)
	}

	targetPath := filepath.Join(draft.CurrentPath, text)
	targetPath, err := ensureDirectory(targetPath)
	if err != nil || !pathWithinRoot(draft.BrowseRoot, targetPath) {
		_ = r.bot.SendMessage(ctx, chatID, nil, "Can't find that folder here.")
		return r.sendBrowsePrompt(ctx, chatID, draft)
	}

	draft.CurrentPath = targetPath
	r.saveDraft(draft)
	return r.sendBrowsePrompt(ctx, chatID, draft)
}

func (r *Runtime) buildStartChoices(userID int64) ([]startChoice, error) {
	sessions, err := r.store.ListSessions()
	if err != nil {
		return nil, err
	}

	var recent state.Session
	found := false
	for i := len(sessions) - 1; i >= 0; i-- {
		session := sessions[i]
		if session.OwnerUserID != userID || strings.TrimSpace(session.WorkRoot) == "" {
			continue
		}
		recent = session
		found = true
		break
	}

	if !found {
		home, err := ensureDirectory(userHomeDir())
		if err != nil {
			return nil, err
		}
		return []startChoice{{
			Label:    "Home",
			Path:     home,
			RootPath: home,
		}}, nil
	}

	recentPath, err := ensureDirectory(recent.WorkRoot)
	if err != nil {
		home, homeErr := ensureDirectory(userHomeDir())
		if homeErr != nil {
			return nil, homeErr
		}
		return []startChoice{{
			Label:    "Home",
			Path:     home,
			RootPath: home,
		}}, nil
	}

	parentPath := filepath.Dir(recentPath)
	if parentPath == "." || parentPath == string(filepath.Separator) {
		parentPath = recentPath
	}
	parentPath, err = ensureDirectory(parentPath)
	if err != nil {
		parentPath = recentPath
	}

	choices := []startChoice{{
		Label:    filepath.Base(recentPath),
		Path:     recentPath,
		RootPath: parentPath,
	}}

	siblings, err := listDirectories(parentPath)
	if err == nil {
		for _, sibling := range siblings {
			if filepath.Clean(sibling.Path) == filepath.Clean(recentPath) {
				continue
			}
			choices = append(choices, startChoice{
				Label:    sibling.Name,
				Path:     sibling.Path,
				RootPath: parentPath,
			})
			if len(choices) >= 4 {
				break
			}
		}
	}

	return choices, nil
}

func renderProviderPrompt(cfg config.Config) (string, telegram.InlineKeyboardMarkup) {
	text := "Which agent do you want for this session?"
	rows := [][]telegram.InlineKeyboardButton{}
	providerRow := []telegram.InlineKeyboardButton{}
	if strings.TrimSpace(cfg.Providers.CodexCommand) != "" {
		providerRow = append(providerRow, telegram.InlineKeyboardButton{Text: "Codex", CallbackData: "wiz:provider:codex"})
	}
	if strings.TrimSpace(cfg.Providers.ClaudeCommand) != "" {
		providerRow = append(providerRow, telegram.InlineKeyboardButton{Text: "Claude", CallbackData: "wiz:provider:claude"})
	}
	if len(providerRow) > 0 {
		rows = append(rows, providerRow)
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "Cancel", CallbackData: "wiz:cancel"}})
	return text, telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func renderStartPrompt(draft generalDraft) (string, telegram.InlineKeyboardMarkup) {
	text := "Where should we start looking?"
	rows := [][]telegram.InlineKeyboardButton{}
	row := []telegram.InlineKeyboardButton{}
	for i, choice := range draft.StartChoices {
		row = append(row, telegram.InlineKeyboardButton{
			Text:         choice.Label,
			CallbackData: fmt.Sprintf("wiz:start:%d", i),
		})
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	if len(draft.StartChoices) > 0 && draft.StartChoices[0].Label != "Home" {
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "More Places", CallbackData: "wiz:start:more"}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "Cancel", CallbackData: "wiz:cancel"}})
	return text, telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func renderBrowsePrompt(draft generalDraft) (string, telegram.InlineKeyboardMarkup) {
	lines := []string{
		fmt.Sprintf("You’re in %s.", draft.CurrentPath),
		"Send a folder name, send .. to go up, or tap Choose Here.",
	}

	rows := [][]telegram.InlineKeyboardButton{}
	entries, err := browseChildren(draft.CurrentPath)
	if err == nil {
		row := []telegram.InlineKeyboardButton{}
		for i, entry := range entries {
			row = append(row, telegram.InlineKeyboardButton{
				Text:         entry.Name,
				CallbackData: fmt.Sprintf("wiz:browse:open:%d", i),
			})
			if len(row) == 2 {
				rows = append(rows, row)
				row = nil
			}
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}

	actionRow := []telegram.InlineKeyboardButton{}
	if draft.BrowseRoot != "" && filepath.Clean(draft.CurrentPath) != filepath.Clean(draft.BrowseRoot) {
		actionRow = append(actionRow, telegram.InlineKeyboardButton{Text: "Go Up", CallbackData: "wiz:browse:up"})
	}
	actionRow = append(actionRow, telegram.InlineKeyboardButton{Text: "Choose Here", CallbackData: "wiz:browse:choose"})
	rows = append(rows, actionRow)
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "Cancel", CallbackData: "wiz:cancel"}})

	return strings.Join(lines, "\n"), telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func renderTaskPrompt(draft generalDraft) string {
	return fmt.Sprintf("Agent: %s\nFolder: %s\nSend the task now to create the topic and launch the session.", draft.Provider, draft.WorkRoot)
}

func (r *Runtime) sendBrowsePrompt(ctx context.Context, chatID int64, draft generalDraft) error {
	text, markup := renderBrowsePrompt(draft)
	_, err := r.bot.SendMessageCard(ctx, chatID, nil, text, markup)
	return err
}

func (r *Runtime) sendTaskPrompt(ctx context.Context, chatID int64, draft generalDraft) error {
	return r.bot.SendMessage(ctx, chatID, nil, renderTaskPrompt(draft))
}

func (d generalDraft) launchPrompt() string {
	return strings.TrimSpace(d.Task)
}

func browseChildren(path string) ([]browseEntry, error) {
	entries, err := listDirectories(path)
	if err != nil {
		return nil, err
	}
	if len(entries) > browseButtonLimit {
		entries = entries[:browseButtonLimit]
	}
	return entries, nil
}

func listDirectories(path string) ([]browseEntry, error) {
	dirEntries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	entries := make([]browseEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.HasPrefix(name, ".") || !entry.IsDir() {
			continue
		}
		entries = append(entries, browseEntry{
			Name: name,
			Path: filepath.Join(path, name),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

func browseParentPath(rootPath, currentPath string) string {
	rootPath = filepath.Clean(rootPath)
	currentPath = filepath.Clean(currentPath)
	if rootPath == "" || currentPath == "" || rootPath == currentPath {
		return currentPath
	}

	parentPath := filepath.Dir(currentPath)
	if !pathWithinRoot(rootPath, parentPath) {
		return rootPath
	}
	return parentPath
}

func pathWithinRoot(rootPath, targetPath string) bool {
	rootPath = filepath.Clean(rootPath)
	targetPath = filepath.Clean(targetPath)
	if rootPath == "" || targetPath == "" {
		return false
	}

	rel, err := filepath.Rel(rootPath, targetPath)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func ensureDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory")
	}
	return filepath.Clean(abs), nil
}

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
