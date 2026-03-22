package telegram

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       UpdateMessage  `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type UpdateMessage struct {
	MessageID int `json:"message_id"`
	UserID    int64
	ChatID    int64
	ThreadID  int
	Text      string `json:"text"`
}

type CallbackQuery struct {
	ID         string
	FromUserID int64
	Data       string
	Message    UpdateMessage
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}
