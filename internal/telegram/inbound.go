package telegram

// PTYWriter writes text into a running agent process.
type PTYWriter interface {
	Write(sessionID string, text string) error
}

// SessionLookup finds a session by its Telegram topic thread ID.
type SessionLookup interface {
	ByThreadID(threadID int) (sessionID string, found bool)
}

type InboundRouter struct {
	Auth            *Authorizer
	Sessions        SessionLookup
	PTY             PTYWriter
	GeneralThreadID int
}

// HandleUpdate processes one incoming Telegram update.
// Returns ("", nil) for ignored/unauthorized messages.
// Returns (reply, nil) for commands that produce a response.
func (r *InboundRouter) HandleUpdate(userID int64, threadID int, text string) (string, error) {
	if threadID == r.GeneralThreadID {
		return r.handleGeneralTopic(userID, text)
	}

	sessionID, found := r.Sessions.ByThreadID(threadID)
	if !found {
		return "", nil
	}

	if !r.Auth.CanSendCommand(userID) {
		return "", nil
	}

	if err := r.PTY.Write(sessionID, text); err != nil {
		return "", err
	}

	return "", nil
}

func (r *InboundRouter) handleGeneralTopic(userID int64, text string) (string, error) {
	if !r.Auth.CanSendCommand(userID) {
		return "", nil
	}

	switch text {
	case "status":
		return "status: ok", nil
	default:
		return "", nil
	}
}
