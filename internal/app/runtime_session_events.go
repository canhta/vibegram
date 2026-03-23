package app

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/canhta/vibegram/internal/events"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

type streamObserver struct {
	r       *Runtime
	ctx     context.Context
	chatID  int64
	thread  int
	session *state.Session
	run     state.Run
	deduper *events.Deduper

	mu  sync.Mutex
	err error
}

func newStreamObserver(r *Runtime, ctx context.Context, chatID int64, threadID int, session *state.Session, run state.Run) *streamObserver {
	return &streamObserver{
		r:       r,
		ctx:     ctx,
		chatID:  chatID,
		thread:  threadID,
		session: session,
		run:     run,
		deduper: events.NewDeduper(),
	}
}

func (o *streamObserver) OnLine(line string) {
	if o == nil {
		return
	}
	if err := o.r.deliverStreamLine(o.ctx, o.chatID, o.thread, o.session, o.run, o.deduper, line); err != nil {
		o.mu.Lock()
		if o.err == nil {
			o.err = err
		}
		o.mu.Unlock()
	}
}

func (o *streamObserver) Err() error {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.err
}

func (r *Runtime) deliverSessionResult(ctx context.Context, chatID int64, threadID int, session *state.Session, run state.Run, result codexprovider.SessionResult, deduper *events.Deduper, allowAutoReply bool) error {
	normalized := r.normalizedResultEvents(*session, run, result)
	actionable, hasActionable := firstActionableEvent(normalized)

	for _, event := range normalized {
		if deduper != nil && !deduper.MarkIfNew(event) {
			continue
		}
		if err := r.projectSessionEvent(ctx, chatID, threadID, session, event, r.policy != nil); err != nil {
			return err
		}
		if err := r.sendEvent(ctx, chatID, threadID, session, event); err != nil {
			return err
		}
	}

	message := strings.TrimSpace(result.Message)
	if len(normalized) == 0 {
		if message == "" {
			return r.sendSessionMessage(ctx, chatID, threadID, session, "Sent to "+session.Provider+". No visible reply yet.")
		}
		if err := r.sendSessionMessage(ctx, chatID, threadID, session, message); err != nil {
			return err
		}
		if allowAutoReply {
			return r.maybeAutoReply(ctx, chatID, threadID, session, run, message)
		}
		return nil
	}

	if message != "" && !messageCoveredByEvents(message, normalized) {
		if err := r.sendSessionMessage(ctx, chatID, threadID, session, message); err != nil {
			return err
		}
	}

	if allowAutoReply && hasActionable {
		return r.maybeAutoReplyForEvent(ctx, chatID, threadID, session, run, actionable)
	}
	return nil
}

func (r *Runtime) deliverStreamLine(ctx context.Context, chatID int64, threadID int, session *state.Session, run state.Run, deduper *events.Deduper, line string) error {
	normalized := r.normalizedProviderStreamLine(*session, run, line)
	for _, event := range normalized {
		if deduper != nil && !deduper.MarkIfNew(event) {
			continue
		}
		if err := r.projectSessionEvent(ctx, chatID, threadID, session, event, r.policy != nil); err != nil {
			return fmt.Errorf("project streamed event: %w", err)
		}
		if err := r.sendEvent(ctx, chatID, threadID, session, event); err != nil {
			return fmt.Errorf("send streamed event: %w", err)
		}
	}
	return nil
}

func (r *Runtime) normalizedResultEvents(session state.Session, run state.Run, result codexprovider.SessionResult) []events.NormalizedEvent {
	switch session.Provider {
	case "codex":
		return normalizedCodexTranscriptEvents(string(session.ID), string(run.ID), result.RawOutput)
	default:
		return nil
	}
}

func (r *Runtime) normalizedProviderStreamLine(session state.Session, run state.Run, line string) []events.NormalizedEvent {
	switch session.Provider {
	case "codex":
		return normalizedCodexTranscriptEvents(string(session.ID), string(run.ID), line)
	default:
		return nil
	}
}

func normalizedCodexTranscriptEvents(sessionID, runID, rawOutput string) []events.NormalizedEvent {
	rawOutput = strings.TrimSpace(rawOutput)
	if rawOutput == "" {
		return nil
	}

	adapter := codexprovider.New(sessionID, runID)
	deduper := events.NewDeduper()
	var normalized []events.NormalizedEvent
	scanner := bufio.NewScanner(strings.NewReader(rawOutput))
	base := time.Now().UTC()
	lineIndex := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}

		observation, ok, err := adapter.ParseTranscriptLine(base.Add(time.Duration(lineIndex)*time.Millisecond), []byte(line))
		lineIndex++
		if err != nil || !ok {
			continue
		}

		event, err := events.Normalize(observation.Observation)
		if err != nil || !deduper.MarkIfNew(event) {
			continue
		}
		normalized = append(normalized, event)
	}
	return normalized
}

func (r *Runtime) sendEvent(ctx context.Context, chatID int64, threadID int, session *state.Session, event events.NormalizedEvent) error {
	router := telegram.Router{
		ChatID:          chatID,
		GeneralThreadID: 1,
		SessionThreadID: threadID,
	}
	for _, destination := range router.Route(event) {
		var targetThread *int
		if destination.Type == telegram.TopicSession {
			targetThread = &destination.ThreadID
		}
		if destination.Type == telegram.TopicSession {
			if err := r.sendSessionMessage(ctx, destination.ChatID, destination.ThreadID, session, telegram.Render(event)); err != nil {
				return err
			}
			continue
		}
		if err := r.bot.SendMessage(ctx, destination.ChatID, targetThread, telegram.Render(event)); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) sendSessionMessage(ctx context.Context, chatID int64, threadID int, session *state.Session, text string) error {
	if err := r.bot.SendMessage(ctx, chatID, &threadID, text); err != nil {
		if isTopicGoneError(err) {
			return r.retireMissingSessionTopic(ctx, chatID, session, err)
		}
		return err
	}
	return nil
}

func (r *Runtime) retireMissingSessionTopic(ctx context.Context, chatID int64, session *state.Session, cause error) error {
	if session == nil {
		return nil
	}

	wasMapped := session.SessionTopicID > 1
	if wasMapped {
		r.mu.Lock()
		delete(r.sessionsByThread, int(session.SessionTopicID))
		r.mu.Unlock()
	}

	if wasMapped {
		session.SessionTopicID = 0
	}
	session.Status = state.SessionStatusFailed
	session.LastBlocker = cause.Error()
	session.EscalationState = state.EscalationStateNeeded
	snap, err := r.loadSessionSnapshot(*session)
	if err != nil {
		return fmt.Errorf("load session snapshot: %w", err)
	}
	snap.Status = string(session.Status)
	snap.LastBlocker = session.LastBlocker
	snap.ApplySupportDecision(state.SupportStateEscalated, cause.Error(), true)
	session.ApplySnapshot(snap)
	if err := r.store.SaveSnapshot(string(session.ID), snap); err != nil {
		return fmt.Errorf("save missing topic snapshot: %w", err)
	}
	if err := r.store.SaveSession(*session); err != nil {
		return fmt.Errorf("retire missing session topic: %w", err)
	}

	if wasMapped {
		label := cleanupLabel(0, []state.Session{*session})
		_ = r.bot.SendMessage(ctx, chatID, nil, fmt.Sprintf("Session topic missing: %s. Removed it from active routing.", label))
	}
	return r.refreshGeneralControlCardIfPresent(ctx, chatID)
}

func firstActionableEvent(eventsList []events.NormalizedEvent) (events.NormalizedEvent, bool) {
	for _, event := range eventsList {
		if event.EventType == events.EventTypeQuestion || event.EventType == events.EventTypeBlocked {
			return event, true
		}
	}
	return events.NormalizedEvent{}, false
}

func messageCoveredByEvents(message string, eventsList []events.NormalizedEvent) bool {
	message = strings.TrimSpace(message)
	if message == "" {
		return false
	}
	for _, event := range eventsList {
		if strings.TrimSpace(event.Summary) == message {
			return true
		}
	}
	return false
}
