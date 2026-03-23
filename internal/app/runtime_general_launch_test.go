package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/events"
	"github.com/canhta/vibegram/internal/policy"
	codexprovider "github.com/canhta/vibegram/internal/providers/codex"
	"github.com/canhta/vibegram/internal/roles"
	"github.com/canhta/vibegram/internal/state"
	"github.com/canhta/vibegram/internal/telegram"
)

func TestRuntimeGeneralWizardLaunchPersistsRunBeforeProviderFinishes(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	storeDir := t.TempDir()
	store := state.NewStore(storeDir)
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "assistant final message",
		},
		startRelease: make(chan struct{}),
		startCalled:  make(chan struct{}, 1),
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		codex,
		nil,
		nil,
		nil,
	)

	runGeneralWizardToTaskEntryPrompt(t, rt, bot, 1001)

	done := make(chan error, 1)
	go func() {
		done <- rt.HandleUpdate(context.Background(), telegram.Update{
			UpdateID: 22,
			Message: telegram.UpdateMessage{
				UserID:   1001,
				ChatID:   -1001234567890,
				ThreadID: 1,
				Text:     "build health check",
			},
		})
	}()

	select {
	case <-codex.startCalled:
	case <-time.After(1 * time.Second):
		t.Fatal("start runner was not called")
	}

	runs, err := os.ReadDir(filepath.Join(storeDir, "runs"))
	if err != nil {
		t.Fatalf("ReadDir(runs) error = %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run file count = %d, want 1 before provider finishes", len(runs))
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("HandleUpdate(launch) error = %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("HandleUpdate(launch) did not return while provider was still starting")
	}

	close(codex.startRelease)
	rt.Wait()
}

func TestShortTopicCodeUsesLastFourDigitsOfSessionID(t *testing.T) {
	got := shortTopicCode("ses_1774171206463341651")
	if got != "1651" {
		t.Fatalf("shortTopicCode() = %q, want %q", got, "1651")
	}
}

func TestTopicNameForDraftUsesFolderProviderAndShortCode(t *testing.T) {
	got := topicNameForDraft(generalDraft{
		Provider: "codex",
		WorkRoot: "/Users/canh/Desktop",
	}, "1651")
	if got != "Desktop codex 1651" {
		t.Fatalf("topicNameForDraft() = %q, want %q", got, "Desktop codex 1651")
	}
}

func TestRuntimeGeneralWizardCanLaunchClaude(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{}
	claude := &fakeCodexSessionRunner{
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "claude-session-123",
			Message:           "claude final message",
		},
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand:  "/usr/local/bin/codex",
				ClaudeCommand: "/usr/local/bin/claude",
			},
		},
		store,
		bot,
		codex,
		claude,
		nil,
		nil,
	)

	runGeneralWizardToChosenProviderTaskEntry(t, rt, bot, 1001, "claude")
	sendTaskFromGeneral(t, rt, 1001, "ship marketing page")

	waitForRuntime(t, func() bool { return claude.startPrompt == "ship marketing page" }, "claude launch")
	if codex.startPrompt != "" {
		t.Fatalf("codex.startPrompt = %q, want empty when claude is selected", codex.startPrompt)
	}
	if claude.startWorkDir != projectX {
		t.Fatalf("claude.startWorkDir = %q, want %q", claude.startWorkDir, projectX)
	}
	waitForRuntime(t, func() bool {
		sent := bot.sentSnapshot()
		return len(sent) >= 6 && sent[len(sent)-1].text == "claude final message"
	}, "claude final message")
}

func TestRuntimeGeneralWizardLaunchAutoRepliesToSafeQuestion(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "Which test framework should I use?",
			RawOutput: strings.Join([]string{
				`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"go test ./...","aggregated_output":"","exit_code":0,"status":"completed"}}`,
				`{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"Which test framework should I use?"}}`,
			}, "\n"),
		},
		resumeResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "I'll use the Go standard library testing package.",
			RawOutput: strings.Join([]string{
				`{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":"I'll use the Go standard library testing package."}}`,
			}, "\n"),
		},
	}
	engine := &fakePolicyEngine{
		decision: policy.PolicyDecision{
			Action:  roles.ActionReply,
			Message: "Use Go's standard library testing package.",
		},
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		codex,
		nil,
		engine,
		nil,
	)

	runGeneralWizardToTaskEntryPrompt(t, rt, bot, 1001)
	sendTaskFromGeneral(t, rt, 1001, "choose test framework")

	waitForRuntime(t, func() bool { return engine.called }, "support-role policy call")
	if engine.lastEvent.EventType != events.EventTypeQuestion {
		t.Fatalf("EventType = %q, want %q", engine.lastEvent.EventType, events.EventTypeQuestion)
	}
	waitForRuntime(t, func() bool { return codex.resumePrompt == "Use Go's standard library testing package." }, "support-role follow-up resume")
	waitForRuntime(t, func() bool { return len(bot.sentSnapshot()) >= 8 }, "support-role message fanout")
	if codex.resumeWorkDir != projectX {
		t.Fatalf("resumeWorkDir = %q, want %q", codex.resumeWorkDir, projectX)
	}
	sent := bot.sentSnapshot()
	if !hasSentText(sent, "Tool: shell — go test ./...") {
		t.Fatalf("sent messages = %+v, want tool message", sent)
	}
	if !hasSentText(sent, "Question: Which test framework should I use?") {
		t.Fatalf("sent messages = %+v, want question message", sent)
	}
	if !hasSentContaining(sent, "Support replied in project-x codex ") || !hasSentContaining(sent, "Use Go's standard library testing package.") {
		t.Fatalf("sent messages = %+v, want General support awareness", sent)
	}
	if !hasSentText(sent, "Agent reply: Use Go's standard library testing package.") {
		t.Fatalf("sent messages = %+v, want agent reply note", sent)
	}
	if !hasSentText(sent, "I'll use the Go standard library testing package.") {
		t.Fatalf("sent messages = %+v, want follow-up message", sent)
	}
}

func TestRuntimeGeneralWizardLaunchRendersFilteredCodexEvents(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "Which test framework should I use?",
			RawOutput: strings.Join([]string{
				`{"type":"item.completed","item":{"id":"item_0","type":"command_execution","command":"/bin/zsh -lc \"sed -n '1,220p' /Users/canh/.codex/superpowers/skills/using-superpowers/SKILL.md\"","aggregated_output":"","exit_code":0,"status":"completed"}}`,
				`{"type":"item.completed","item":{"id":"item_00","type":"command_execution","command":"/bin/zsh -lc 'rg --files .'","aggregated_output":"","exit_code":0,"status":"completed"}}`,
				`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"go test ./...","aggregated_output":"","exit_code":0,"status":"completed"}}`,
				`{"type":"item.completed","item":{"id":"item_15","type":"agent_message","text":"Some of what we're working on might be easier to explain if I can show it to you in a web browser. Want to try it? (Requires opening a local URL)"}}`,
				`{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"Which test framework should I use?"}}`,
			}, "\n"),
		},
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		codex,
		nil,
		nil,
		nil,
	)

	runGeneralWizardToTaskEntryPrompt(t, rt, bot, 1001)
	sendTaskFromGeneral(t, rt, 1001, "choose test framework")

	waitForRuntime(t, func() bool { return len(bot.sentSnapshot()) >= 8 }, "filtered codex launch messages")
	sent := bot.sentSnapshot()
	if sent[len(sent)-2].text != "Tool: shell — go test ./..." {
		t.Fatalf("tool message = %q", sent[len(sent)-2].text)
	}
	if sent[len(sent)-1].text != "Question: Which test framework should I use?" {
		t.Fatalf("question message = %q", sent[len(sent)-1].text)
	}
	if hasSentText(sent, "Question: Some of what we're working on might be easier to explain if I can show it to you in a web browser. Want to try it? (Requires opening a local URL)") {
		t.Fatal("browser-offer noise should not be rendered")
	}
}

func TestRuntimeGeneralWizardLaunchUpdatesSessionHeaderCardFromSupportProjection(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "Which file should I edit?",
			RawOutput: strings.Join([]string{
				`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"Which file should I edit?"}}`,
			}, "\n"),
		},
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		codex,
		nil,
		nil,
		nil,
	)

	runGeneralWizardToTaskEntryPrompt(t, rt, bot, 1001)
	sendTaskFromGeneral(t, rt, 1001, "choose file")

	rt.Wait()

	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	var session state.Session
	for _, candidate := range sessions {
		if candidate.ID != "ses_prior" {
			session = candidate
			break
		}
	}
	if session.ID == "" {
		t.Fatal("launched session was not persisted")
	}
	if session.SupportState != state.SupportStateAskHuman {
		t.Fatalf("SupportState = %q, want %q", session.SupportState, state.SupportStateAskHuman)
	}
	if session.SupportDecisionSummary != "Which file should I edit?" {
		t.Fatalf("SupportDecisionSummary = %q, want %q", session.SupportDecisionSummary, "Which file should I edit?")
	}
	if !session.HumanActionNeeded {
		t.Fatal("HumanActionNeeded = false, want true")
	}
	if session.SessionHeaderMessageID == 0 {
		t.Fatal("SessionHeaderMessageID = 0, want a persistent header card")
	}

	sent := bot.sentSnapshot()
	headerFound := false
	for _, msg := range sent {
		if msg.markup == nil {
			continue
		}
		if strings.Contains(msg.text, "Support: idle") && strings.Contains(msg.text, "Human action: no") {
			headerFound = true
			break
		}
	}
	if !headerFound {
		t.Fatalf("sent messages did not include the initial session header card: %+v", sent)
	}

	edited := bot.editedSnapshot()
	if len(edited) == 0 {
		t.Fatal("editedSnapshot() = 0, want header card update")
	}
	if !strings.Contains(edited[len(edited)-1].text, "Support: ask human") {
		t.Fatalf("edited header card = %q, want support state", edited[len(edited)-1].text)
	}
	if !strings.Contains(edited[len(edited)-1].text, "Human action: yes") {
		t.Fatalf("edited header card = %q, want human action flag", edited[len(edited)-1].text)
	}
}

func TestRuntimeGeneralWizardLaunchMarksHeaderFailedWhenStartFails(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{
		startErr: errors.New("provider offline"),
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		codex,
		nil,
		nil,
		nil,
	)

	runGeneralWizardToTaskEntryPrompt(t, rt, bot, 1001)
	sendTaskFromGeneral(t, rt, 1001, "finish setup")

	rt.Wait()

	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	var session state.Session
	for _, candidate := range sessions {
		if candidate.ID != "ses_prior" {
			session = candidate
			break
		}
	}
	if session.ID == "" {
		t.Fatal("launched session was not persisted")
	}
	if session.Status != state.SessionStatusFailed {
		t.Fatalf("Status = %q, want %q", session.Status, state.SessionStatusFailed)
	}
	if session.SupportState != state.SupportStateEscalated {
		t.Fatalf("SupportState = %q, want %q", session.SupportState, state.SupportStateEscalated)
	}
	if session.SupportDecisionSummary != "provider offline" {
		t.Fatalf("SupportDecisionSummary = %q, want %q", session.SupportDecisionSummary, "provider offline")
	}
	if !session.HumanActionNeeded {
		t.Fatal("HumanActionNeeded = false, want true")
	}

	edited := bot.editedSnapshot()
	if len(edited) == 0 {
		t.Fatal("editedSnapshot() = 0, want header card update")
	}
	last := edited[len(edited)-1].text
	if !strings.Contains(last, "State: failed / planning") {
		t.Fatalf("edited header card = %q, want failed state", last)
	}
	if !strings.Contains(last, "Support: escalated") {
		t.Fatalf("edited header card = %q, want escalated support state", last)
	}
	if !strings.Contains(last, "Support decision: provider offline") {
		t.Fatalf("edited header card = %q, want failure reason", last)
	}
	if !strings.Contains(last, "Human action: yes") {
		t.Fatalf("edited header card = %q, want human action flag", last)
	}

	sent := bot.sentSnapshot()
	if !hasSentText(sent, "start failed: provider offline") {
		t.Fatalf("sent messages = %+v, want start failure message", sent)
	}
	if !hasSentContaining(sent, "Support escalated in project-x codex ") || !hasSentText(sent, "start failed: provider offline") {
		t.Fatalf("sent messages = %+v, want General escalation awareness", sent)
	}
}

func TestRuntimeGeneralWizardLaunchRoutesBlockerResolvedToGeneralAndSession(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "I resolved the blocker by adding the missing API token, and I can continue now.",
			RawOutput: strings.Join([]string{
				`{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"I resolved the blocker by adding the missing API token, and I can continue now."}}`,
			}, "\n"),
		},
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		codex,
		nil,
		nil,
		nil,
	)

	runGeneralWizardToTaskEntryPrompt(t, rt, bot, 1001)
	sendTaskFromGeneral(t, rt, 1001, "finish setup")

	wantText := "Blocker resolved: I resolved the blocker by adding the missing API token, and I can continue now."
	waitForRuntime(t, func() bool { return hasSentText(bot.sentSnapshot(), wantText) }, "blocker resolved fanout")

	var generalCount, sessionCount int
	for _, msg := range bot.sentSnapshot() {
		if msg.text != wantText {
			continue
		}
		if msg.threadID == nil {
			generalCount++
			continue
		}
		if *msg.threadID == 42 {
			sessionCount++
		}
	}
	if generalCount != 1 {
		t.Fatalf("general blocker resolved count = %d, want 1", generalCount)
	}
	if sessionCount != 1 {
		t.Fatalf("session blocker resolved count = %d, want 1", sessionCount)
	}
}

func TestRuntimeGeneralWizardLaunchStreamsFilteredCodexEventsBeforeProviderFinishes(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{
		startStreamLines: []string{
			`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"go test ./...","aggregated_output":"","exit_code":0,"status":"completed"}}`,
			`{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"Which test framework should I use?"}}`,
		},
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "Which test framework should I use?",
			RawOutput: strings.Join([]string{
				`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"go test ./...","aggregated_output":"","exit_code":0,"status":"completed"}}`,
				`{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"Which test framework should I use?"}}`,
			}, "\n"),
		},
		startRelease: make(chan struct{}),
		startCalled:  make(chan struct{}, 1),
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		codex,
		nil,
		nil,
		nil,
	)

	runGeneralWizardToTaskEntryPrompt(t, rt, bot, 1001)

	done := make(chan error, 1)
	go func() {
		done <- rt.HandleUpdate(context.Background(), telegram.Update{
			UpdateID: 23,
			Message: telegram.UpdateMessage{
				UserID:   1001,
				ChatID:   -1001234567890,
				ThreadID: 1,
				Text:     "choose test framework",
			},
		})
	}()

	select {
	case <-codex.startCalled:
	case <-time.After(1 * time.Second):
		t.Fatal("start runner was not called")
	}

	waitForRuntime(t, func() bool {
		sent := bot.sentSnapshot()
		return hasSentText(sent, "Tool: shell — go test ./...") &&
			hasSentText(sent, "Question: Which test framework should I use?")
	}, "streamed filtered codex events before provider exit")

	close(codex.startRelease)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("HandleUpdate() error = %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("HandleUpdate() did not finish after start release")
	}

	rt.Wait()
}

func TestRuntimeGeneralWizardLaunchReportsCreateTopicFailureWithoutCrashing(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{createTopicErr: fmt.Errorf("telegram api error 400: Bad Request: not enough rights to create a topic")}
	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		&fakeCodexSessionRunner{},
		nil,
		nil,
		nil,
	)

	runGeneralWizardToTaskEntryPrompt(t, rt, bot, 1001)
	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 9,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "build health check",
		},
	}); err != nil {
		t.Fatalf("HandleUpdate() error = %v, want nil", err)
	}
	if len(bot.sent) == 0 {
		t.Fatal("sent messages = 0, want launch failed notice")
	}
	if !strings.Contains(strings.ToLower(bot.sent[len(bot.sent)-1].text), "launch failed") {
		t.Fatalf("failure message = %q, want launch failed notice", bot.sent[len(bot.sent)-1].text)
	}
}

func TestRuntimeHandleGeneralSlashNewSupportsBotMention(t *testing.T) {
	projectRoot := t.TempDir()
	projectX := filepath.Join(projectRoot, "project-x")
	if err := os.Mkdir(projectX, 0o755); err != nil {
		t.Fatalf("Mkdir(project-x) error = %v", err)
	}

	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := store.SaveSession(state.Session{
		ID:             "ses_prior",
		OwnerUserID:    1001,
		GeneralTopicID: 1,
		SessionTopicID: 42,
		WorkRoot:       projectX,
		Status:         state.SessionStatusDone,
		Phase:          state.SessionPhaseWaiting,
	}); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{nextThreadID: 42}
	codex := &fakeCodexSessionRunner{
		startResult: codexprovider.SessionResult{
			ProviderSessionID: "thread-123",
			Message:           "assistant final message",
		},
	}

	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
			Providers: config.ProviderConfig{
				CodexCommand: "/usr/local/bin/codex",
			},
		},
		store,
		bot,
		codex,
		nil,
		nil,
		nil,
	)

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 40,
		Message: telegram.UpdateMessage{
			UserID:   1001,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/new@vibeloop_bot",
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(/new@mention) error = %v", err)
	}

	if len(bot.sent) != 1 {
		t.Fatalf("sent messages = %d, want provider prompt", len(bot.sent))
	}
	if !strings.Contains(bot.sent[0].text, "Which agent do you want for this session?") {
		t.Fatalf("provider prompt = %q", bot.sent[0].text)
	}
}

func runGeneralWizardToChosenProviderTaskEntry(t *testing.T, rt *Runtime, bot *fakeBotClient, userID int64, provider string) {
	t.Helper()

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 4,
		Message: telegram.UpdateMessage{
			UserID:   userID,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     "/new",
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(/new) error = %v", err)
	}

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 5,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-provider",
			FromUserID: userID,
			Data:       "wiz:provider:" + provider,
			Message: telegram.UpdateMessage{
				MessageID: bot.sent[len(bot.sent)-1].messageID,
				UserID:    userID,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(provider) error = %v", err)
	}

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 6,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-start-choice",
			FromUserID: userID,
			Data:       "wiz:start:0",
			Message: telegram.UpdateMessage{
				MessageID: bot.sent[len(bot.sent)-1].messageID,
				UserID:    userID,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(start choice) error = %v", err)
	}

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 7,
		CallbackQuery: &telegram.CallbackQuery{
			ID:         "cb-choose-here",
			FromUserID: userID,
			Data:       "wiz:browse:choose",
			Message: telegram.UpdateMessage{
				MessageID: bot.sent[len(bot.sent)-1].messageID,
				UserID:    userID,
				ChatID:    -1001234567890,
				ThreadID:  1,
			},
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(choose here) error = %v", err)
	}

}

func hasSentText(messages []sentMessage, want string) bool {
	for _, message := range messages {
		if message.text == want {
			return true
		}
	}
	return false
}

func hasSentContaining(messages []sentMessage, want string) bool {
	for _, message := range messages {
		if strings.Contains(message.text, want) {
			return true
		}
	}
	return false
}

func sendTaskFromGeneral(t *testing.T, rt *Runtime, userID int64, task string) {
	t.Helper()

	if err := rt.HandleUpdate(context.Background(), telegram.Update{
		UpdateID: 9,
		Message: telegram.UpdateMessage{
			UserID:   userID,
			ChatID:   -1001234567890,
			ThreadID: 1,
			Text:     task,
		},
	}); err != nil {
		t.Fatalf("HandleUpdate(task) error = %v", err)
	}
}
