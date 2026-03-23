package app

import (
	"context"
	"testing"

	"github.com/canhta/vibegram/internal/config"
	"github.com/canhta/vibegram/internal/state"
)

func TestDesiredSessionTopicTitleUsesDynamicStateIcons(t *testing.T) {
	tests := []struct {
		name    string
		session state.Session
		want    string
	}{
		{
			name: "running",
			session: state.Session{
				ID:       "ses_1774230019339001903",
				Provider: "codex",
				Status:   state.SessionStatusRunning,
				WorkRoot: "/tmp/vibegram",
				LastGoal: "cleanup mismatch",
			},
			want: "🟢 [codex] · vibegram · cleanup mismatch · #1903",
		},
		{
			name: "question needing human",
			session: state.Session{
				ID:                "ses_1774230019339001903",
				Provider:          "codex",
				Status:            state.SessionStatusRunning,
				WorkRoot:          "/tmp/vibegram",
				LastGoal:          "cleanup mismatch",
				LastQuestion:      "should General get high-signal awareness?",
				HumanActionNeeded: true,
			},
			want: "❓ [codex] · vibegram · cleanup mismatch · #1903",
		},
		{
			name: "blocked needing human",
			session: state.Session{
				ID:                "ses_1774230019339001903",
				Provider:          "codex",
				Status:            state.SessionStatusBlocked,
				WorkRoot:          "/tmp/vibegram",
				LastGoal:          "cleanup mismatch",
				LastBlocker:       "message thread not found",
				HumanActionNeeded: true,
			},
			want: "⛔ [codex] · vibegram · cleanup mismatch · #1903",
		},
		{
			name: "idle waiting phase",
			session: state.Session{
				ID:       "ses_1774230019339001903",
				Provider: "codex",
				Status:   state.SessionStatusRunning,
				Phase:    state.SessionPhaseWaiting,
				WorkRoot: "/tmp/vibegram",
				LastGoal: "cleanup mismatch",
			},
			want: "⚪ [codex] · vibegram · cleanup mismatch · #1903",
		},
		{
			name: "done",
			session: state.Session{
				ID:       "ses_1774230019339001903",
				Provider: "codex",
				Status:   state.SessionStatusDone,
				WorkRoot: "/tmp/vibegram",
				LastGoal: "cleanup mismatch",
			},
			want: "✅ [codex] · vibegram · cleanup mismatch · #1903",
		},
		{
			name: "failed",
			session: state.Session{
				ID:       "ses_1774230019339001903",
				Provider: "codex",
				Status:   state.SessionStatusFailed,
				WorkRoot: "/tmp/vibegram",
				LastGoal: "cleanup mismatch",
			},
			want: "❌ [codex] · vibegram · cleanup mismatch · #1903",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := desiredSessionTopicTitle(tt.session); got != tt.want {
				t.Fatalf("desiredSessionTopicTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUpsertSessionHeaderCardRenamesTopicWhenStatusChanges(t *testing.T) {
	store := state.NewStore(t.TempDir())
	if err := store.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	session := state.Session{
		ID:                     "ses_1774230019339001903",
		ActiveRunID:            "run_1",
		Provider:               "codex",
		GeneralTopicID:         1,
		SessionTopicID:         42,
		SessionTopicTitle:      "🟢 [codex] · vibegram · cleanup mismatch · #1903",
		SessionHeaderMessageID: 99,
		Status:                 state.SessionStatusFailed,
		Phase:                  state.SessionPhasePlanning,
		LastGoal:               "cleanup mismatch",
		LastBlocker:            "context canceled",
		HumanActionNeeded:      true,
		WorkRoot:               "/tmp/vibegram",
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}

	bot := &fakeBotClient{}
	rt := NewRuntime(
		config.Config{
			Telegram: config.TelegramConfig{
				ForumChatID: -1001234567890,
				AdminIDs:    []int64{1001},
			},
		},
		store,
		bot,
		nil,
		nil,
		nil,
		nil,
	)

	if err := rt.upsertSessionHeaderCard(context.Background(), -1001234567890, 42, &session, true); err != nil {
		t.Fatalf("upsertSessionHeaderCard() error = %v", err)
	}

	editedTopics := bot.editedTopicNamesSnapshot()
	if len(editedTopics) != 1 {
		t.Fatalf("editedTopicNames = %+v, want 1 topic rename", editedTopics)
	}
	if editedTopics[0].name != "❌ [codex] · vibegram · cleanup mismatch · #1903" {
		t.Fatalf("edited topic name = %q, want failed title", editedTopics[0].name)
	}

	updated, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if updated.SessionTopicTitle != "❌ [codex] · vibegram · cleanup mismatch · #1903" {
		t.Fatalf("SessionTopicTitle = %q, want failed title", updated.SessionTopicTitle)
	}
}
