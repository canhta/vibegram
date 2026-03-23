package app

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

const (
	maxTopicTitleLen     = 64
	maxTopicProviderLen  = 10
	maxTopicFolderLen    = 18
	minTopicTaskPartLen  = 7
	startingTopicIcon    = "🟡"
	runningTopicIcon     = "🟢"
	questionTopicIcon    = "❓"
	blockedTopicIcon     = "⛔"
	idleTopicIcon        = "⚪"
	doneTopicIcon        = "✅"
	failedTopicIcon      = "❌"
	defaultTopicProvider = "agent"
	defaultTopicFolder   = "session"
)

func topicNameForDraft(draft generalDraft, shortCode string) string {
	return renderSessionTopicTitle(topicTitleSpec{
		icon:      startingTopicIcon,
		provider:  strings.TrimSpace(draft.Provider),
		folder:    sessionFolderLabel(draft.WorkRoot),
		task:      topicTaskLabel(draft.launchPrompt()),
		shortCode: shortCode,
	})
}

func derivedTopicTitle(session state.Session) string {
	return desiredSessionTopicTitle(session)
}

func desiredSessionTopicTitle(session state.Session) string {
	return renderSessionTopicTitle(topicTitleSpec{
		icon:      sessionTopicIcon(session),
		provider:  strings.TrimSpace(session.Provider),
		folder:    sessionFolderLabel(session.WorkRoot),
		task:      topicTaskLabel(session.LastGoal),
		shortCode: shortTopicCode(session.ID),
	})
}

type topicTitleSpec struct {
	icon      string
	provider  string
	folder    string
	task      string
	shortCode string
}

func renderSessionTopicTitle(spec topicTitleSpec) string {
	icon := strings.TrimSpace(spec.icon)
	if icon == "" {
		icon = runningTopicIcon
	}

	provider := compactTopicText(spec.provider)
	if provider == "" {
		provider = defaultTopicProvider
	}
	provider = telegram.TruncateText(provider, maxTopicProviderLen)

	folder := compactTopicText(spec.folder)
	if folder == "" {
		folder = defaultTopicFolder
	}
	folder = telegram.TruncateText(folder, maxTopicFolderLen)

	shortCode := strings.TrimSpace(spec.shortCode)
	if shortCode == "" {
		shortCode = "0000"
	}

	base := fmt.Sprintf("%s [%s] · %s", icon, provider, folder)
	suffix := " · #" + shortCode

	task := compactTopicText(spec.task)
	taskPart := ""
	if task != "" {
		taskRoom := maxTopicTitleLen - runeLen(base) - runeLen(suffix) - runeLen(" · ")
		if taskRoom >= minTopicTaskPartLen {
			taskPart = " · " + telegram.TruncateText(task, taskRoom)
		}
	}

	title := base + taskPart + suffix
	if runeLen(title) <= maxTopicTitleLen {
		return title
	}

	overflow := runeLen(title) - maxTopicTitleLen
	folderMax := runeLen(folder) - overflow
	if folderMax < 6 {
		folderMax = 6
	}
	base = fmt.Sprintf("%s [%s] · %s", icon, provider, telegram.TruncateText(folder, folderMax))
	title = base + taskPart + suffix
	if runeLen(title) <= maxTopicTitleLen {
		return title
	}

	return trimTopicTitle(title, maxTopicTitleLen)
}

func sessionTopicIcon(session state.Session) string {
	switch strings.TrimSpace(string(session.Status)) {
	case string(state.SessionStatusDone):
		return doneTopicIcon
	case string(state.SessionStatusFailed):
		return failedTopicIcon
	}

	if session.HumanActionNeeded {
		if strings.TrimSpace(session.LastBlocker) != "" || strings.TrimSpace(string(session.Status)) == string(state.SessionStatusBlocked) {
			return blockedTopicIcon
		}
		if strings.TrimSpace(session.LastQuestion) != "" {
			return questionTopicIcon
		}
	}

	if strings.TrimSpace(string(session.Phase)) == string(state.SessionPhaseWaiting) {
		return idleTopicIcon
	}
	if strings.TrimSpace(string(session.Status)) == string(state.SessionStatusBlocked) {
		return blockedTopicIcon
	}
	return runningTopicIcon
}

func topicTaskLabel(goal string) string {
	return compactTopicText(goal)
}

func compactTopicText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return strings.Join(strings.Fields(text), " ")
}

func trimTopicTitle(text string, max int) string {
	text = compactTopicText(text)
	if runeLen(text) <= max {
		return text
	}

	suffixIdx := strings.LastIndex(text, " · #")
	if suffixIdx <= 0 {
		return telegram.TruncateText(text, max)
	}

	suffix := text[suffixIdx:]
	prefix := strings.TrimSpace(text[:suffixIdx])
	room := max - runeLen(suffix) - runeLen(" ")
	if room <= 0 {
		return telegram.TruncateText(text, max)
	}
	return strings.TrimSpace(telegram.TruncateText(prefix, room) + " " + suffix)
}

func runeLen(text string) int {
	return utf8.RuneCountInString(text)
}

func (r *Runtime) syncSessionTopicTitle(ctx context.Context, chatID int64, threadID int, session *state.Session) error {
	if session == nil || threadID <= 1 || r.bot == nil {
		return nil
	}

	title := desiredSessionTopicTitle(*session)
	if title == "" || strings.TrimSpace(session.SessionTopicTitle) == title {
		return nil
	}

	if err := r.bot.EditForumTopic(ctx, chatID, threadID, title); err != nil {
		if isTopicGoneError(err) {
			return r.retireMissingSessionTopic(ctx, chatID, session, err)
		}
		return fmt.Errorf("edit forum topic: %w", err)
	}

	session.SessionTopicTitle = title
	if err := r.store.SaveSession(*session); err != nil {
		return fmt.Errorf("save session topic title: %w", err)
	}
	return nil
}
