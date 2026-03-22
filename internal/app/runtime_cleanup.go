package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/canhta/vibegram/internal/state"
)

func (r *Runtime) cleanupTopics(ctx context.Context, chatID int64, raw string) error {
	threadIDs, sessions, err := r.cleanupTargets(raw)
	if err != nil {
		return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
	}

	deleted := 0
	for _, threadID := range threadIDs {
		if err := r.bot.DeleteForumTopic(ctx, chatID, threadID); err != nil {
			if !isTopicGoneError(err) {
				return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
			}
		}

		r.mu.Lock()
		delete(r.sessionsByThread, threadID)
		r.mu.Unlock()
		deleted++
	}

	for _, session := range sessions {
		if session.SessionTopicID > 1 {
			r.mu.Lock()
			delete(r.sessionsByThread, int(session.SessionTopicID))
			r.mu.Unlock()
		}
	}

	for _, session := range sessions {
		if session.ActiveRunID != "" {
			if err := r.store.DeleteRun(session.ActiveRunID); err != nil && !isNotFound(err) {
				return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
			}
		}
		if err := r.store.DeleteSession(session.ID); err != nil && !isNotFound(err) {
			return r.bot.SendMessage(ctx, chatID, nil, "cleanup failed: "+err.Error())
		}
	}

	return r.bot.SendMessage(ctx, chatID, nil, fmt.Sprintf("cleanup: deleted %d topic(s)", deleted))
}

func parseThreadIDs(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	threadIDs := make([]int, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}

		threadID, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid topic id %q", value)
		}
		threadIDs = append(threadIDs, threadID)
	}

	if len(threadIDs) == 0 {
		return nil, fmt.Errorf("at least one topic id is required")
	}
	return threadIDs, nil
}

func (r *Runtime) cleanupTargets(raw string) ([]int, []state.Session, error) {
	sessions, err := r.store.ListSessions()
	if err != nil {
		return nil, nil, fmt.Errorf("list sessions: %w", err)
	}

	raw = strings.TrimSpace(raw)
	if raw == "all" {
		seen := make(map[int]struct{}, len(sessions))
		threadIDs := make([]int, 0, len(sessions))
		cleanupSessions := make([]state.Session, 0, len(sessions))
		for _, session := range sessions {
			if session.SessionTopicID <= 1 {
				continue
			}

			threadID := int(session.SessionTopicID)
			if _, ok := seen[threadID]; !ok {
				seen[threadID] = struct{}{}
				threadIDs = append(threadIDs, threadID)
			}
			cleanupSessions = append(cleanupSessions, session)
		}
		return threadIDs, cleanupSessions, nil
	}

	threadIDs, err := parseThreadIDs(raw)
	if err != nil {
		return nil, nil, err
	}
	threadIDs = dedupeThreadIDs(threadIDs)

	byThread := make(map[int][]state.Session, len(sessions))
	for _, session := range sessions {
		byThread[int(session.SessionTopicID)] = append(byThread[int(session.SessionTopicID)], session)
	}

	cleanupSessions := make([]state.Session, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		cleanupSessions = append(cleanupSessions, byThread[threadID]...)
	}
	return threadIDs, cleanupSessions, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, state.ErrNotFound)
}

func isTopicGoneError(err error) bool {
	return strings.Contains(err.Error(), "TOPIC_ID_INVALID")
}

func dedupeThreadIDs(threadIDs []int) []int {
	seen := make(map[int]struct{}, len(threadIDs))
	deduped := make([]int, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		if _, ok := seen[threadID]; ok {
			continue
		}
		seen[threadID] = struct{}{}
		deduped = append(deduped, threadID)
	}
	return deduped
}
