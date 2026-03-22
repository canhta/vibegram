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
- If the user is asking to create a new coding session, tell them to use /start <goal>.
- If the user is asking for runtime state, tell them to use /status.
- If the user is asking to delete or clean session topics, tell them to use /cleanup.
- Do not claim that you executed commands or changed runtime state unless explicit tool output says so.`

type SupportResponder struct {
	caller Caller
	rules  string
}

func NewSupportResponder(caller Caller) *SupportResponder {
	return &SupportResponder{caller: caller, rules: supportChatRules}
}

func (r *SupportResponder) Reply(ctx context.Context, text string) (string, error) {
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
