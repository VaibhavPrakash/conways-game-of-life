# CLAUDE.md

This file provides guidance for AI assistants working in this repository.

## Project Overview

This repository was originally a Conway's Game of Life cellular automaton simulator (frontend, TypeScript/HTML). It has since been **repurposed** as the home for a **Prediction Markets Backend** platform—a Go monolith that aggregates prediction markets from multiple venues (Kuru on Monad, Polymarket on Polygon) and exposes a unified REST API.

The current repository state is architecture-first: `ARCHITECTURE.md` is the single source of truth for the planned backend implementation.

---

## Repository Structure

```
conways-game-of-life/
├── CLAUDE.md           # This file — AI assistant guidance
├── ARCHITECTURE.md     # Comprehensive backend architecture specification
└── .gitignore          # Node.js / general dev environment ignores
```

> **Note:** All original Conway's Game of Life source files (TypeScript, HTML, CSS, package.json) were removed in commit `cd4fe42` as part of the pivot to backend architecture.

---

## Architecture Summary

See `ARCHITECTURE.md` for the full specification. Key points:

| Aspect | Detail |
|--------|--------|
| **Language** | Go (monolith) |
| **Database** | PostgreSQL (via `sqlc` + `pgx`) |
| **Router** | `gin` or `chi` |
| **Auth** | Privy JWT verification middleware |
| **Blockchain** | Monad (USDC), Polygon (Polymarket) |
| **Venues** | Kuru (Monad) + Polymarket (Polygon) |

### Internal Go Packages

The architecture defines these internal packages:

- `api` — HTTP handlers and routing
- `auth` — Privy JWT middleware
- `market` — Market listing and metadata
- `trade` — Order placement and venue abstraction
- `portfolio` — User positions and PnL
- `bridge` — Cross-chain bridge abstraction (multi-provider)
- `venue` — Venue interface + Kuru/Polymarket implementations
- `user` — User profile management
- `sync` — Background market/position sync workers
- `db` — sqlc-generated database layer

### Key Data Flows

1. **Browse Markets** — Sync worker fetches from Kuru + Polymarket → stored in DB → served via `/markets` endpoint
2. **Place Trade** — User calls `/trade` → backend routes to appropriate venue → on-chain tx submitted → position synced
3. **Portfolio View** — User calls `/portfolio` → positions + PnL aggregated from DB

### Background Workers

- Market sync (periodic fetch from venues)
- Position sync (reconcile on-chain state)
- Bridge monitor (track cross-chain transactions)
- PnL snapshots
- Market resolution

---

## Development Workflow

### Current State

The repository contains only documentation—no Go code exists yet. When implementation begins:

1. Initialize the Go module: `go mod init github.com/VaibhavPrakash/conways-game-of-life`
2. Set up directory structure per `ARCHITECTURE.md`
3. Generate DB layer: `sqlc generate`
4. Generate contract bindings: `abigen`

### Git Conventions

- **Branch naming**: `claude/<description>-<session-id>` for AI-driven work
- **Commit signing**: All commits are SSH-signed (configured via `commit.gpgsign=true`)
- **Remote**: `origin` → `http://local_proxy@127.0.0.1:35169/git/VaibhavPrakash/conways-game-of-life`
- **Push**: Always use `git push -u origin <branch-name>`

### Branches

| Branch | Purpose |
|--------|---------|
| `master` | Historical Conway's Game of Life code |
| `main` (remote) | Current default branch |
| `claude/add-claude-documentation-1tIbb` | Current AI documentation work |

---

## Planned Tech Stack

When implementing the Go backend, use these tools as specified in `ARCHITECTURE.md`:

| Tool | Purpose |
|------|---------|
| `gin` / `chi` | HTTP routing |
| `pgx` + `sqlc` | PostgreSQL access with type-safe generated queries |
| `abigen` | Go bindings for Solidity contracts |
| `go-order-utils` (Polymarket) | Polymarket order signing |
| Privy SDK / JWT libraries | Auth token verification |
| WebSocket client | Real-time Kuru order book |

---

## Testing Plan

From `ARCHITECTURE.md` (no tests exist yet):

1. **Unit tests** — Each venue implementation mocked
2. **Integration tests** — Polymarket testnet CLOB + Monad testnet
3. **Manual E2E** — Full user flows
4. **Load tests** — Market sync workers under load

When tests are added, run them with: `go test ./...`

---

## Key Conventions for AI Assistants

1. **Read `ARCHITECTURE.md` first** before making any implementation decisions — it is the authoritative design document.
2. **Do not re-introduce** Conway's Game of Life frontend files; those were intentionally removed.
3. **Go package boundaries** — respect the internal package structure defined in the architecture.
4. **Database changes** require updating SQL schema files and regenerating via `sqlc generate`.
5. **Venue abstraction** — new trading venues must implement the `venue.Venue` interface.
6. **Authentication** — all protected endpoints must use Privy JWT middleware; never bypass auth.
7. **Commit all work** with descriptive messages and push to the designated branch.
8. **No force pushes** to `main` or `master`.

---

## Historical Context

| Date | Event |
|------|-------|
| Aug 2023 | Initial Conway's Game of Life implementation (TypeScript + HTML5) |
| Mar 2026 | Repository pivoted to prediction markets backend |
| Mar 2026 | All frontend files removed; `ARCHITECTURE.md` added |
| Mar 2026 | `CLAUDE.md` created (this file) |
