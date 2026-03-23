package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

const generalControlCardCursor = "general_control_card_message_id"

type generalControlCardRefreshResult string

const (
	generalControlCardRefreshCreated   generalControlCardRefreshResult = "created"
	generalControlCardRefreshEdited    generalControlCardRefreshResult = "edited"
	generalControlCardRefreshUnchanged generalControlCardRefreshResult = "unchanged"
)

type generalControlCardSession struct {
	title           string
	provider        string
	status          string
	phase           string
	goal            string
	blocker         string
	question        string
	files           string
	tests           string
	escalation      string
	supportState    string
	supportDecision string
	humanAction     bool
	rank            int
}

func (r *Runtime) refreshGeneralControlCard(ctx context.Context, chatID int64) error {
	_, err := r.refreshGeneralControlCardResult(ctx, chatID)
	return err
}

func (r *Runtime) refreshGeneralControlCardResult(ctx context.Context, chatID int64) (generalControlCardRefreshResult, error) {
	text, markup, err := r.generalControlCardText()
	if err != nil {
		return "", err
	}

	if messageID, found, err := r.generalControlCardMessageID(); err != nil {
		return "", err
	} else if found {
		if err := r.bot.EditMessageCard(ctx, chatID, messageID, text, markup); err == nil {
			return generalControlCardRefreshEdited, nil
		} else if isMessageNotModifiedError(err) {
			return generalControlCardRefreshUnchanged, nil
		}
	}

	messageID, err := r.bot.SendMessageCard(ctx, chatID, nil, text, markup)
	if err != nil {
		return "", err
	}
	if err := r.saveGeneralControlCardMessageID(messageID); err != nil {
		return "", err
	}
	return generalControlCardRefreshCreated, nil
}

func (r *Runtime) refreshGeneralControlCardIfPresent(ctx context.Context, chatID int64) error {
	messageID, found, err := r.generalControlCardMessageID()
	if err != nil {
		return err
	}
	if !found || messageID == 0 {
		return nil
	}
	return r.refreshGeneralControlCard(ctx, chatID)
}

func (r *Runtime) generalControlCardText() (string, telegram.InlineKeyboardMarkup, error) {
	if r.store == nil {
		return renderGeneralControlCardEmpty(), telegram.InlineKeyboardMarkup{}, nil
	}

	sessions, err := r.store.ListSessions()
	if err != nil {
		return "", telegram.InlineKeyboardMarkup{}, fmt.Errorf("list sessions: %w", err)
	}

	entries := make([]generalControlCardSession, 0, len(sessions))
	needsHumanCount := 0
	runningCount := 0
	blockedCount := 0
	for _, session := range sessions {
		entry, active, err := r.generalControlCardSession(session)
		if err != nil {
			return "", telegram.InlineKeyboardMarkup{}, err
		}
		if !active {
			continue
		}
		entries = append(entries, entry)
		if entry.humanAction {
			needsHumanCount++
		}
		switch entry.status {
		case string(state.SessionStatusRunning):
			runningCount++
		case string(state.SessionStatusBlocked):
			blockedCount++
		}
	}

	if len(entries) == 0 {
		return renderGeneralControlCardEmpty(), telegram.InlineKeyboardMarkup{}, nil
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].rank != entries[j].rank {
			return entries[i].rank < entries[j].rank
		}
		if entries[i].title != entries[j].title {
			return entries[i].title < entries[j].title
		}
		return entries[i].provider < entries[j].provider
	})

	lines := []string{
		"General control room",
		fmt.Sprintf("Needs you now: %d | Active: %d | Running: %d | Blocked: %d", needsHumanCount, len(entries), runningCount, blockedCount),
		"",
	}
	for i, entry := range entries {
		lines = append(lines, fmt.Sprintf("%d. %s | %s | %s",
			i+1,
			telegram.TruncateText(entry.title, 72),
			telegram.TruncateText(entry.provider, 24),
			telegram.TruncateText(entry.status, 24),
		))
		if entry.phase != "" {
			lines = append(lines, "   Phase: "+telegram.TruncateText(entry.phase, 48))
		}
		if entry.humanAction {
			lines = append(lines, "   Needs you now: yes")
		}
		if entry.supportState != "" && entry.supportState != string(state.SupportStateIdle) {
			lines = append(lines, "   Support: "+telegram.TruncateText(entry.supportState, 24))
		}
		if entry.supportDecision != "" {
			lines = append(lines, "   Decision: "+telegram.TruncateText(entry.supportDecision, 160))
		}
		if entry.goal != "" {
			lines = append(lines, "   Goal: "+telegram.TruncateText(entry.goal, 160))
		}
		if entry.blocker != "" {
			lines = append(lines, "   Blocker: "+telegram.TruncateText(entry.blocker, 160))
		}
		if entry.question != "" {
			lines = append(lines, "   Question: "+telegram.TruncateText(entry.question, 160))
		}
		if entry.files != "" {
			lines = append(lines, "   Files: "+telegram.TruncateText(entry.files, 160))
		}
		if entry.tests != "" {
			lines = append(lines, "   Tests: "+telegram.TruncateText(entry.tests, 160))
		}
		if entry.escalation != "" && entry.escalation != string(state.EscalationStateNone) {
			lines = append(lines, "   Escalation: "+telegram.TruncateText(entry.escalation, 32))
		}
		if i < len(entries)-1 {
			lines = append(lines, "")
		}
	}

	return telegram.ClampMessageText(strings.Join(lines, "\n")), telegram.InlineKeyboardMarkup{}, nil
}

func (r *Runtime) generalControlCardSession(session state.Session) (generalControlCardSession, bool, error) {
	display := r.displaySession(session)
	entry := generalControlCardSession{
		title:           strings.TrimSpace(display.SessionTopicTitle),
		provider:        strings.TrimSpace(display.Provider),
		status:          sessionDisplayStatusLabel(display),
		phase:           strings.TrimSpace(string(display.Phase)),
		goal:            strings.TrimSpace(display.LastGoal),
		blocker:         strings.TrimSpace(display.LastBlocker),
		question:        strings.TrimSpace(display.LastQuestion),
		files:           strings.TrimSpace(display.RecentFilesSummary),
		tests:           strings.TrimSpace(display.RecentTestsSummary),
		escalation:      strings.TrimSpace(string(display.EscalationState)),
		supportState:    supportStateLabel(display.SupportState),
		supportDecision: strings.TrimSpace(display.SupportDecisionSummary),
		humanAction:     display.HumanActionNeeded,
	}
	if entry.title == "" {
		entry.title = strings.TrimSpace(derivedTopicTitle(display))
	}
	if entry.title == "" {
		entry.title = string(display.ID)
	}
	if entry.provider == "" {
		entry.provider = "unknown"
	}

	snap, err := r.generalControlCardSnapshot(display.ID)
	if err != nil {
		return generalControlCardSession{}, false, err
	}
	if snap != nil {
		display.ApplySnapshot(*snap)
		display = r.displaySession(display)
		entry.status = sessionDisplayStatusLabel(display)
		entry.phase = strings.TrimSpace(string(display.Phase))
		entry.blocker = strings.TrimSpace(display.LastBlocker)
		entry.question = strings.TrimSpace(display.LastQuestion)
		entry.files = strings.TrimSpace(display.RecentFilesSummary)
		entry.tests = strings.TrimSpace(display.RecentTestsSummary)
		entry.escalation = strings.TrimSpace(string(display.EscalationState))
		entry.supportState = supportStateLabel(display.SupportState)
		entry.supportDecision = strings.TrimSpace(display.SupportDecisionSummary)
		entry.humanAction = display.HumanActionNeeded
	}

	entry.rank = generalControlCardRank(entry)
	if !generalControlCardIsActive(entry) {
		return generalControlCardSession{}, false, nil
	}

	return entry, true, nil
}

func (r *Runtime) generalControlCardSnapshot(sessionID state.SessionID) (*state.Snapshot, error) {
	snap, err := r.store.LoadSnapshot(string(sessionID))
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load snapshot for %s: %w", sessionID, err)
	}
	return &snap, nil
}

func (r *Runtime) generalControlCardMessageID() (int, bool, error) {
	if r.store == nil {
		return 0, false, nil
	}

	value, err := r.store.LoadCursor(generalControlCardCursor)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("load general control card cursor: %w", err)
	}
	if value <= 0 {
		return 0, false, nil
	}
	return int(value), true, nil
}

func (r *Runtime) saveGeneralControlCardMessageID(messageID int) error {
	if r.store == nil {
		return nil
	}
	return r.store.SaveCursor(generalControlCardCursor, int64(messageID))
}

func renderGeneralControlCardEmpty() string {
	return strings.Join([]string{
		"General control room",
		"Needs you now: 0 | Active: 0 | Running: 0 | Blocked: 0",
		"",
		"No active sessions.",
		"Use /new to start one.",
	}, "\n")
}

func generalControlCardIsActive(entry generalControlCardSession) bool {
	if entry.humanAction {
		return true
	}
	switch entry.status {
	case "idle", string(state.SessionStatusRunning), string(state.SessionStatusBlocked):
		return true
	case string(state.SessionStatusFailed):
		return entry.escalation != "" && entry.escalation != string(state.EscalationStateNone)
	default:
		return false
	}
}

func generalControlCardRank(entry generalControlCardSession) int {
	if entry.humanAction {
		return 0
	}
	switch entry.status {
	case string(state.SessionStatusBlocked):
		return 1
	case "idle":
		if entry.supportState == string(state.SupportStateReplied) || entry.supportDecision != "" {
			return 2
		}
		return 3
	case string(state.SessionStatusRunning):
		if entry.supportState == string(state.SupportStateReplied) || entry.supportDecision != "" {
			return 2
		}
		return 3
	default:
		return 4
	}
}
