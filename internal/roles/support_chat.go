package roles

import (
	"context"
	"fmt"
	"strings"
)

const supportChatRules = `You are the General-topic support agent for vibegram, a Telegram control room for coding agents.

Behavior:
- Reply conversationally and briefly.
- Help the user think through coding and workflow questions.
- General-topic control actions must stay slash-only.
- If the user is asking to create a new coding session, tell them to use /new.
- If the user is asking for runtime state, tell them to use /status.
- If the user is asking to delete or clean session topics, tell them to use /cleanup.
- Do not claim that you executed commands or changed runtime state unless explicit tool output says so.`

const supportValidationRules = `You are the General-topic draft validator for vibegram.

Behavior:
- Turn a rough coding request plus project context into a cleaner launch brief.
- Stay faithful to the user's intent.
- Use the project context to remove ambiguity, not to invent new work.
- Reply in plain text only, with a concise launch brief.`

type SupportResponder struct {
	caller       Caller
	strongCaller Caller
	rules        string
}

func NewSupportResponder(caller Caller, strongCaller ...Caller) *SupportResponder {
	var strong Caller
	if len(strongCaller) > 0 {
		strong = strongCaller[0]
	}
	return &SupportResponder{caller: caller, strongCaller: strong, rules: supportChatRules}
}

func (r *SupportResponder) Reply(ctx context.Context, text string) (string, error) {
	if reply, ok := deterministicGeneralReply(text); ok {
		return reply, nil
	}

	prompt := fmt.Sprintf(`SYSTEM
%s

USER
%s

TASK
Reply to the user with a concise, helpful answer.`, r.rules, text)

	reply, err := r.caller.Call(ctx, prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(reply), nil
}

func (r *SupportResponder) Validate(ctx context.Context, text string) (string, error) {
	prompt := fmt.Sprintf(`SYSTEM
%s

USER
%s

TASK
Return only the final launch brief.`, supportValidationRules, text)

	caller := r.caller
	if r.strongCaller != nil {
		caller = r.strongCaller
	}

	reply, err := caller.Call(ctx, prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(reply), nil
}

func deterministicGeneralReply(text string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(lower, "new session"), strings.Contains(lower, "start a session"), strings.Contains(lower, "create a session"):
		return "Use /new to start a new session.", true
	case strings.Contains(lower, "status"), strings.Contains(lower, "what is running"), strings.Contains(lower, "what's running"):
		return "Use /status to see the control room.", true
	case strings.Contains(lower, "cleanup"), strings.Contains(lower, "delete topic"), strings.Contains(lower, "remove topic"):
		return "Use /cleanup to choose session topics to delete.", true
	default:
		return "", false
	}
}
