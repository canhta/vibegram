# Telegram Research

This file captures the current Telegram platform constraints that materially affect `vibegram`.

## Forum facts that matter

Official Telegram forum behavior is a strong match for `vibegram`:

- Every forum has a non-deletable General topic with `id=1`.
- To send messages to the General topic, Telegram treats it like a normal supergroup.
- Other topics are message threads and should be treated like threads by the client.

Source:
- [Telegram forums](https://core.telegram.org/api/forum)

Why this matters for `vibegram`:

- the General topic is a true control room
- per-session topics should be treated as thread-like working rooms
- we should not invent a fake "meta topic" abstraction on top of Telegram

## Bot API limits that shape the UX

### Text and callback payloads

- `sendMessage` text is limited to `1-4096` characters after entities parsing.
- `callback_data` is limited to `1-64 bytes`.

Sources:
- [sendMessage](https://core.telegram.org/bots/api#sendmessage)
- [InlineKeyboardButton](https://core.telegram.org/bots/api#inlinekeyboardbutton)

Implications:

- Telegram messages must be compact, card-like summaries
- callback payloads must use compact IDs and server-side state lookup
- raw terminal streams should never be the default UI

### Topic routing

- `message_thread_id` is the Bot API field used to target forum topics.

Source:
- [sendMessage](https://core.telegram.org/bots/api#sendmessage)

Implications:

- General topic and session topics can share one bot and one chat
- topic routing is a first-class transport primitive, not a hack

### Web Apps

- `web_app` buttons in inline keyboards are available only in private chats between a user and the bot.

Source:
- [InlineKeyboardButton](https://core.telegram.org/bots/api#inlinekeyboardbutton)

Implications:

- `vibegram` should not depend on a Telegram Mini App for the core group/forum workflow
- group topics are for alerts, control, and concise collaboration, not app-grade UI

## File and artifact limits

- Bots can send files up to `50 MB` with `sendDocument`.
- `getFile` downloads are limited to `20 MB`.
- A local Bot API server raises these limits substantially, including uploads up to `2000 MB` and downloads without a size limit.

Sources:
- [sendDocument](https://core.telegram.org/bots/api#senddocument)
- [File / getFile](https://core.telegram.org/bots/api#file)
- [Local Bot API server](https://core.telegram.org/bots/api#using-a-local-bot-api-server)

Implications:

- session timelines should prefer summaries over large attachments
- if `vibegram` later sends heavy artifacts like bundle logs or traces, it may need either:
  - a local Bot API server
  - external artifact storage
  - stronger truncation and summarization rules

## Recommended Telegram stance

The current best Telegram posture for `vibegram` is:

```text
Forum supergroup
  -> General topic for control room duties
  -> per-session topics for detailed but filtered session output
  -> no Mini App dependency
  -> no raw transcript spam
```

That remains the best balance of what Telegram supports well today versus what it makes awkward.
