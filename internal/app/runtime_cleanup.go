package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

type cleanupTarget struct {
	ThreadID int
	Label    string
	Sessions []state.Session
}

func (r *Runtime) showCleanupPicker(ctx context.Context, chatID int64) error {
	targets, err := r.cleanupTargets()
	if err != nil {
		return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
	}
	if len(targets) == 0 {
		return r.bot.SendMessage(ctx, chatID, nil, "cleanup: no session topics")
	}

	rows := make([][]telegram.InlineKeyboardButton, 0, len(targets)+1)
	for _, target := range targets {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text:         target.Label,
			CallbackData: fmt.Sprintf("cleanup:topic:%d", target.ThreadID),
		}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{
		Text:         "All",
		CallbackData: "cleanup:all",
	}})

	_, err = r.bot.SendMessageCard(ctx, chatID, nil, "Select session topics to delete.", telegram.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	})
	return err
}

func (r *Runtime) handleCleanupCallback(ctx context.Context, chatID int64, data string) error {
	switch {
	case data == "cleanup:all":
		targets, err := r.cleanupTargets()
		if err != nil {
			return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
		}
		return r.cleanupTargetsBySelection(ctx, chatID, targets)

	case strings.HasPrefix(data, "cleanup:topic:"):
		threadID, err := strconv.Atoi(strings.TrimPrefix(data, "cleanup:topic:"))
		if err != nil {
			return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: invalid topic id")
		}
		target, ok, err := r.cleanupTargetByThread(threadID)
		if err != nil {
			return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
		}
		if !ok {
			return r.bot.SendMessage(ctx, chatID, nil, "cleanup: topic already gone")
		}
		return r.cleanupTargetsBySelection(ctx, chatID, []cleanupTarget{target})

	default:
		return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: unknown action")
	}
}

func (r *Runtime) cleanupTargetsBySelection(ctx context.Context, chatID int64, targets []cleanupTarget) error {
	deleted := 0
	for _, target := range targets {
		if err := r.bot.DeleteForumTopic(ctx, chatID, target.ThreadID); err != nil {
			if !isTopicGoneError(err) {
				return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
			}
		} else {
			deleted++
		}

		r.mu.Lock()
		delete(r.sessionsByThread, target.ThreadID)
		r.mu.Unlock()

		for _, session := range target.Sessions {
			if session.SessionTopicID > 1 {
				r.mu.Lock()
				delete(r.sessionsByThread, int(session.SessionTopicID))
				r.mu.Unlock()
			}
		}

		for _, session := range target.Sessions {
			if session.ActiveRunID != "" {
				if err := r.store.DeleteRun(session.ActiveRunID); err != nil && !isNotFound(err) {
					return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
				}
			}
			if err := r.store.DeleteSession(session.ID); err != nil && !isNotFound(err) {
				return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
			}
		}
	}

	return r.bot.SendMessage(ctx, chatID, nil, fmt.Sprintf("cleanup: deleted %d topic(s)", len(targets)))
}

func (r *Runtime) cleanupTargetByThread(threadID int) (cleanupTarget, bool, error) {
	targets, err := r.cleanupTargets()
	if err != nil {
		return cleanupTarget{}, false, err
	}
	for _, target := range targets {
		if target.ThreadID == threadID {
			return target, true, nil
		}
	}
	return cleanupTarget{}, false, nil
}

func (r *Runtime) cleanupTargets() ([]cleanupTarget, error) {
	sessions, err := r.store.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	byThread := make(map[int][]state.Session, len(sessions))
	for _, session := range sessions {
		if session.SessionTopicID <= 1 {
			continue
		}
		threadID := int(session.SessionTopicID)
		byThread[threadID] = append(byThread[threadID], session)
	}

	targets := make([]cleanupTarget, 0, len(byThread))
	for threadID, sessions := range byThread {
		targets = append(targets, cleanupTarget{
			ThreadID: threadID,
			Label:    cleanupLabel(threadID, sessions),
			Sessions: sessions,
		})
	}

	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Label == targets[j].Label {
			return targets[i].ThreadID < targets[j].ThreadID
		}
		return targets[i].Label < targets[j].Label
	})

	return targets, nil
}

func cleanupLabel(threadID int, sessions []state.Session) string {
	for _, session := range sessions {
		if title := strings.TrimSpace(session.SessionTopicTitle); title != "" {
			return title
		}
		if title := derivedTopicTitle(session); title != "" {
			return title
		}
		base := strings.TrimSpace(filepath.Base(session.WorkRoot))
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	return fmt.Sprintf("Topic %d", threadID)
}

func derivedTopicTitle(session state.Session) string {
	if strings.TrimSpace(session.WorkRoot) == "" || strings.TrimSpace(session.Provider) == "" || strings.TrimSpace(string(session.ID)) == "" {
		return ""
	}
	return topicNameForDraft(generalDraft{
		Provider: session.Provider,
		WorkRoot: session.WorkRoot,
	}, shortTopicCode(session.ID))
}

func isNotFound(err error) bool {
	return errors.Is(err, state.ErrNotFound)
}

func isTopicGoneError(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "topic_id_invalid") || strings.Contains(text, "message thread not found")
}
