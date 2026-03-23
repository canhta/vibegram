package roles

import (
	"context"
	"testing"
)

func TestSupportResponderReplyShortCircuitsCommonCommands(t *testing.T) {
	caller := &mockCaller{response: "LLM reply should not be used"}
	responder := NewSupportResponder(caller)

	reply, err := responder.Reply(context.Background(), "how do I start a new session here?")
	if err != nil {
		t.Fatalf("Reply() error = %v", err)
	}
	if reply != "Use /new to start a new session." {
		t.Fatalf("reply = %q, want slash guidance", reply)
	}
	if caller.calls != 0 {
		t.Fatalf("caller.calls = %d, want 0", caller.calls)
	}
}

func TestSupportResponderValidateUsesStrongCallerWhenConfigured(t *testing.T) {
	cheap := &mockCaller{response: "cheap"}
	strong := &mockCaller{response: "strong"}
	responder := NewSupportResponder(cheap, strong)

	reply, err := responder.Validate(context.Background(), "rewrite this launch brief")
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if reply != "strong" {
		t.Fatalf("reply = %q, want strong", reply)
	}
	if cheap.calls != 0 {
		t.Fatalf("cheap.calls = %d, want 0", cheap.calls)
	}
	if strong.calls != 1 {
		t.Fatalf("strong.calls = %d, want 1", strong.calls)
	}
}
