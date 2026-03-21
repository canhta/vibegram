# Diagrams

This document collects the core system diagrams for `vibegram`.

It is intentionally visual and compact. The prose details live in:

- [Architecture](./architecture.md)
- [Telegram Model](./telegram-model.md)
- [Provider Model](./provider-model.md)
- [Session Context](./session-context.md)
- [Automation Safety](./automation-safety.md)

## 1. System Overview

```mermaid
flowchart TD
    TG["Telegram Forum"]
    GT["General Topic<br/>control room"]
    ST["Session Topics<br/>working rooms"]

    DAEMON["vibegram daemon"]
    RUNNER["Runner<br/>direct PTY first"]
    ADAPTERS["Provider adapters"]
    EVENTS["Event normalizer"]
    SNAPSHOT["Rolling snapshot store"]
    RETRIEVAL["Local retrieval index<br/>(later slice)"]
    POLICY["Policy engine"]
    ROLES["Support role executor"]

    CLAUDE["Claude Code"]
    CODEX["Codex"]

    TG --> GT
    TG --> ST

    GT --> DAEMON
    ST --> DAEMON

    DAEMON --> RUNNER
    DAEMON --> ADAPTERS
    DAEMON --> EVENTS
    DAEMON --> SNAPSHOT
    DAEMON --> RETRIEVAL
    DAEMON --> POLICY
    DAEMON --> ROLES

    ADAPTERS --> CLAUDE
    ADAPTERS --> CODEX
```

## 2. Identity Model

```mermaid
flowchart LR
    TOPIC["Telegram session topic"]
    SESSION["app session_id"]
    RUN["run_id"]
    META["provider metadata<br/>provider session ID / PID / transcript path"]

    TOPIC --> SESSION --> RUN --> META
```

Why this matters:

- the Telegram topic stays stable
- a provider run can restart or resume underneath it
- runtime mechanics do not become user-facing identity

## 3. Telegram Topic Split

```mermaid
flowchart TD
    HUMAN["Human"]
    GENERAL["General topic"]
    SESSION["Session topic"]
    BOARD["attention cards<br/>waiting / blocked / done / approval"]

    HUMAN --> GENERAL
    HUMAN --> SESSION

    GENERAL --> BOARD
    GENERAL -->|"create session"| SESSION
    GENERAL -->|"blocked / failed / done / critical"| HUMAN
    SESSION -->|"important events"| HUMAN
    SESSION -->|"auto-reply notes"| HUMAN
    SESSION -->|"escalation context"| HUMAN
```

## 4. Provider Signal Priority

```mermaid
flowchart LR
    CH["Claude hooks"] --> CN["Claude normalizer input"]
    CT["Claude transcript"] --> CN
    CP["Claude PTY fallback"] --> CN

    XT["Codex transcript"] --> XN["Codex normalizer input"]
    XP["Codex PTY fallback"] --> XN

    CN --> E["normalized events"]
    XN --> E
```

Priority rules:

- Claude: hooks first, transcript second, PTY fallback third
- Codex: transcript first, PTY fallback second

## 5. Event Flow

```mermaid
flowchart TD
    RAW["provider observation"]
    NORMALIZE["normalize + dedupe"]
    SNAP["update rolling snapshot"]
    ROUTE{"policy decision"}

    GENERAL["render to General topic"]
    SESSION["render to session topic"]
    ROLE["invoke support role"]
    NOOP["no-op"]

    RAW --> NORMALIZE --> SNAP --> ROUTE
    ROUTE --> GENERAL
    ROUTE --> SESSION
    ROUTE --> ROLE
    ROUTE --> NOOP
```

## 6. Auto-Reply and Escalation

```mermaid
sequenceDiagram
    participant Main as Main agent
    participant V as vibegram
    participant M as Session state and future memory
    participant R as Support role
    participant T as Telegram
    participant H as Human

    Main->>V: blocked / question / done signal
    V->>M: load snapshot and retrieved decisions
    V->>R: bounded role call
    R-->>V: reply | escalate | noop

    alt safe auto-reply
        V->>Main: send direct reply
        V->>T: note in session topic
    else escalate
        V->>T: alert in General topic
        V->>T: context in session topic
        H->>V: human decision
        V->>Main: send human-approved direction
    else noop
        V->>T: concise status note if needed
    end
```

## 7. Human Teaching Loop

```mermaid
flowchart TD
    HUMAN["Human correction or override"]
    DECISION["decision markdown"]
    INDEX["local retrieval index refresh"]
    FUTURE["future retrieval improves role context"]
    PROMOTION["promotion candidate"]
    EVAL["eval gate"]
    RULE["promote into stable rules"]

    HUMAN --> DECISION --> INDEX --> FUTURE --> PROMOTION --> EVAL --> RULE
```

Important constraint:

- the system may capture teaching automatically
- it must not auto-promote new rules without passing evals

## 8. Runtime Ownership

```mermaid
flowchart TD
    SYSTEMD["systemd"]
    DAEMON["vibegram daemon"]
    RUNNER["direct runner"]
    TMUX["optional tmux adapter later"]
    AGENTS["provider child processes"]

    SYSTEMD --> DAEMON
    DAEMON --> RUNNER
    DAEMON -. optional .-> TMUX
    RUNNER --> AGENTS
    TMUX --> AGENTS
```

Default operational stance:

- `systemd` owns uptime
- direct runner is the normal path
- `tmux` is optional, not required

## 9. Trust Boundary and Elevation

```mermaid
flowchart TD
    TP["Trusted policy<br/>rules, config, human-approved decisions"]
    TS["Trusted system state<br/>session, run, counters, routing"]
    UE["Untrusted evidence<br/>transcript, tool output, files, web content"]

    POLICY["Policy engine"]
    CHECK["source-sink risk check"]
    SANDBOX["sandbox profile"]
    ACTION{"reply, escalate, or noop"}

    HUMAN["Human approval"]
    MAIN["Main agent"]

    TP --> POLICY
    TS --> POLICY
    UE --> POLICY
    POLICY --> CHECK --> SANDBOX --> ACTION
    ACTION --> MAIN
    ACTION --> HUMAN
```

Design rule:

- untrusted evidence may influence reasoning
- it may not become authority
- elevated permissions require explicit approval or static policy

## 10. Approval Packet Flow

```mermaid
sequenceDiagram
    participant Main as Main agent
    participant V as vibegram
    participant G as General topic
    participant H as Human approver

    Main->>V: request needs elevation
    V->>G: create approval packet
    H->>G: approve or deny
    G->>V: explicit decision with scope and duration
    V->>Main: continue or refuse
```
