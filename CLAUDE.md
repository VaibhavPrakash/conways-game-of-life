# CLAUDE.md

This file provides guidance for AI assistants working in this repository.

## Project Overview

This repository is the home for a **Prediction Markets Backend** platform — a Go
monolith that aggregates prediction markets from multiple venues (Kuru on Monad,
Polymarket on Polygon) and exposes a unified REST API.

The project began as Conway's Game of Life (TypeScript/HTML), was pivoted in
March 2026, and now contains:

- A comprehensive architecture spec (`ARCHITECTURE.md`)
- A set of agreed UX changes (`suggested-changes.md`)
- A **working Polymarket latency prototype** (`cmd/latency/`, `internal/`)

---

## Repository Structure

```
conways-game-of-life/
├── CLAUDE.md               # This file — AI assistant guidance
├── ARCHITECTURE.md         # Full backend architecture specification
├── PLAN.md                 # Polymarket latency prototype plan
├── suggested-changes.md    # Agreed UX improvements from architecture review
├── trading-ux-options.md   # Bridging & trade execution options analysis
├── go.mod                  # Go module: github.com/VaibhavPrakash/conways-game-of-life
├── cmd/
│   └── latency/
│       └── main.go         # CLI entrypoint for latency testing
└── internal/
    ├── polymarket/
    │   ├── auth.go          # L1/L2 CLOB auth (EIP-712 + HMAC-SHA256)
    │   └── client.go        # CLOB REST client + EIP-712 order building/signing
    ├── relay/
    │   └── relay.go         # Relay bridge client (quote, execute, poll)
    ├── timing/
    │   └── timing.go        # Stopwatch + aggregate stats for latency measurement
    └── wallet/
        └── wallet.go        # Ethereum key management, tx signing, ERC-20 helpers
```

> **Note:** All original Conway's Game of Life source files were removed in
> commit `cd4fe42`. Do not re-introduce them.

---

## What Exists Today vs. What Is Planned

### Implemented (Latency Prototype)

The latency prototype is a **throwaway CLI tool** — no database, no HTTP server,
no auth middleware. It measures end-to-end latency for:
`Monad USDC → Relay bridge → Polygon USDC.e → Polymarket CLOB order`

**`internal/wallet`** — Ethereum wallet utilities
- `FromPrivateKey(hex)` — load wallet from private key
- `BalanceOf`, `Allowance`, `Approve` — ERC-20 helpers
- `SendTx`, `WaitForTx` — transaction submission and receipt polling
- `FormatUSDC` / `ParseUSDC` — 6-decimal USDC conversion helpers
- Constants: `MonadChainID=143`, `PolygonChainID=137`, RPC URLs, token addresses

**`internal/relay`** — Relay bridge client (`https://api.relay.link`)
- `NewClient(apiKey)` — create client
- `GetQuote(ctx, QuoteRequest)` — POST `/quote/v2`
- `GetStepTxData(step)` — extract tx data from quote step
- `PollStatus(ctx, requestID, interval, timeout)` — poll `/intents/status/v3`

**`internal/polymarket/auth.go`** — Polymarket CLOB authentication
- `DeriveAPICredentials(ctx, privateKey, address)` — L1 EIP-712 auth → fetch API credentials
- `(*APICredentials).SignL2Request(method, path, body)` — HMAC-SHA256 L2 request signing

**`internal/polymarket/client.go`** — Polymarket CLOB REST client
- `NewClient(creds, privateKey, address)` — create client
- `BuildAndSignOrder(OrderRequest)` — EIP-712 order construction + signing (no external deps)
- `SubmitOrder(ctx, signedOrder, orderType)` — POST `/order`
- `GetOrderStatus(ctx, orderID)` — GET `/data/order/:id`
- `PollOrderFill(ctx, orderID, interval, timeout)` — poll until filled/cancelled

**`internal/timing`** — Latency measurement utilities
- `New()` / `Start(label)` / `Stop()` — stopwatch per step
- `PrintTable(runLabel)` — render ASCII timing table
- `NewSummary()` / `Summary.Add(tracker)` / `Summary.Print()` — multi-run stats (min/avg/p50/p95/max)

**`cmd/latency/main.go`** — CLI entrypoint
```
Flags:
  --private-key    hex private key (or PRIVATE_KEY env var)
  --market-id      Polymarket token ID (required)
  --amount         USDC amount to trade (default 1.0)
  --side           buy|sell (default buy)
  --price          limit price (default 0.50)
  --runs           number of iterations (default 1)
  --skip-bridge    skip bridge if USDC.e already on Polygon
  --dry-run        measure everything but skip final order submission
  --relay-api-key  optional Relay API key (or RELAY_API_KEY env var)
```

### Not Yet Implemented (From ARCHITECTURE.md)

The full backend (HTTP server, database, auth middleware, background workers)
does not exist yet. The planned packages are:

| Package | Purpose |
|---------|---------|
| `internal/api` | HTTP handlers and routing (gin/chi) |
| `internal/auth` | Privy JWT verification middleware |
| `internal/market` | Market listing, aggregation, caching |
| `internal/trade` | Trade orchestration and venue routing |
| `internal/portfolio` | Positions, PnL, activity feed |
| `internal/bridge` | Bridge abstraction + provider impls |
| `internal/venue/kuru` | Kuru order book client (abigen bindings) |
| `internal/venue/polymarket` | Polymarket CLOB (refactor from prototype) |
| `internal/user` | User profile, wallet associations |
| `internal/sync` | Background market/position sync workers |
| `internal/db` | sqlc-generated database layer |
| `pkg/models` | Shared domain types |

---

## Key Architecture Summary

See `ARCHITECTURE.md` for the full specification.

| Aspect | Detail |
|--------|--------|
| **Language** | Go (monolith) |
| **Database** | PostgreSQL (via `sqlc` + `pgx`) |
| **Router** | `gin` or `chi` |
| **Auth** | Privy JWT verification middleware |
| **Blockchain** | Monad (USDC, chain 143), Polygon (Polymarket, chain 137) |
| **Venues** | Kuru (Monad) + Polymarket (Polygon CLOB) |

### Key Constants (Chain + Contract Addresses)

| Constant | Value |
|----------|-------|
| Monad Chain ID | `143` |
| Monad RPC | `https://rpc.monad.xyz` |
| Polygon Chain ID | `137` |
| Polygon RPC | `https://polygon-rpc.com` |
| USDC on Monad | `0x754704Bc059F8C67012fEd69BC8A327a5aafb603` |
| USDC.e on Polygon | `0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174` |
| CTF Exchange | `0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E` |
| Neg Risk CTF Exchange | `0xC5d563A36AE78145C45a50134d48A1215220f80a` |
| Relay API | `https://api.relay.link` |
| Polymarket CLOB API | `https://clob.polymarket.com` |

---

## Trading UX & Bridging Options (see `trading-ux-options.md`)

Analysis of five options for making Polymarket trades feel as fast as Kuru
trades. Covers bridge-per-trade, pre-funded float, platform liquidity pool,
hybrid routing, and platform-as-broker models. Includes bridge provider
comparison (Relay, Across, deBridge) and speed/custody/complexity trade-offs.

**Current recommendation:** Option B (pre-funded float) + Option A fallback for
MVP. Evaluate Option E (platform-as-broker) for V2.

---

## Agreed UX Changes (see `suggested-changes.md`)

These changes were reviewed and agreed upon. They must be incorporated when
building the full backend:

1. **Real-time order status** — Add `GET /api/v1/orders/:id/stream` (SSE/WebSocket) instead of client polling
2. **Eager Gnosis Safe provisioning** — Deploy Safe at login/onboarding, not on first trade
3. **Trigger-based position sync** — Sync immediately after a fill; don't wait for the 60s background tick
4. **Single unified balance** — `GET /api/v1/user/balances` returns one aggregated total (Monad + Polygon + in-transit)
5. **On-demand market refresh** — `POST /api/v1/markets/:id/refresh` + subscribe to Polymarket WebSocket per viewed market
6. **PostgreSQL full-text search** — `tsvector` GIN index on `markets.question` + `markets.description`
7. **Cached balances with async refresh** — Return cached balance immediately; trigger async RPC update in background

---

## Development Workflows

### Running the Latency Prototype

```bash
# Build
go build ./cmd/latency/

# Dry-run (skip bridge if USDC.e already on Polygon, no real order submitted)
PRIVATE_KEY=0x... ./latency \
  --market-id <polymarket-token-id> \
  --amount 1.0 \
  --price 0.50 \
  --skip-bridge \
  --dry-run

# Full pipeline (requires real USDC on Monad)
PRIVATE_KEY=0x... RELAY_API_KEY=... ./latency \
  --market-id <polymarket-token-id> \
  --amount 1.0 \
  --price 0.50 \
  --runs 3
```

### Starting the Full Backend (Not Yet Implemented)

When implementation begins:

```bash
# Go module already initialized (go.mod exists)

# Add dependencies as needed
go get github.com/gin-gonic/gin
go get github.com/jackc/pgx/v5
# etc.

# Generate DB layer
sqlc generate

# Generate Kuru contract bindings
abigen --abi kuru.abi --pkg kuru --out internal/venue/kuru/bindings.go

# Run server
go run ./cmd/server/
```

### Tests

No tests exist yet. When added, run with:
```bash
go test ./...
```

---

## Git Conventions

- **Branch naming**: `claude/<description>-<session-id>` for AI-driven work
- **Commit signing**: SSH-signed (configured via `commit.gpgsign=true`)
- **Remote**: `origin` → `http://local_proxy@127.0.0.1:35169/git/VaibhavPrakash/conways-game-of-life`
- **Push**: Always use `git push -u origin <branch-name>`
- **No force pushes** to `main` or `master`

### Branches

| Branch | Purpose |
|--------|---------|
| `main` (remote) | Current default branch |
| `master` | Historical Conway's Game of Life code |
| `claude/add-claude-documentation-RpiRg` | Current AI documentation work |

### Commit History (Notable)

| Commit | Description |
|--------|-------------|
| `45ff515` | Merge: Polymarket latency prototype (complete) |
| `8e18ba1` | Add CLI entrypoint for latency testing prototype |
| `fde0544` | Add core packages: wallet, relay, polymarket, timing |
| `f2d3688` | Add suggested UX changes doc from architecture review |
| `266715a` | Add CLAUDE.md with comprehensive codebase documentation |
| `cd4fe42` | Remove Conway's Game of Life files; pivot to backend |
| `eb96e7d` | Add prediction market backend architecture plan |

---

## Key Conventions for AI Assistants

1. **Read `ARCHITECTURE.md` first** — it is the authoritative design document for the full backend.
2. **Read `suggested-changes.md`** — these UX changes are agreed and must be incorporated into the full backend implementation.
3. **Read `trading-ux-options.md`** — bridging and trade execution options for unified UX. Current recommendation is Option B (pre-funded float) for MVP.
4. **Do not re-introduce** Conway's Game of Life frontend files.
5. **Prototype vs. Production** — `internal/polymarket`, `internal/relay`, `internal/wallet`, `internal/timing` are prototype-quality. When building the full backend, refactor these into the proper `internal/venue/polymarket` package structure defined in `ARCHITECTURE.md`.
6. **Go package boundaries** — respect the internal package structure defined in the architecture.
7. **Database changes** require updating SQL schema files and regenerating via `sqlc generate`.
8. **Venue abstraction** — new trading venues must implement the `venue.Venue` interface (defined in `ARCHITECTURE.md`).
9. **Authentication** — all protected endpoints must use Privy JWT middleware; never bypass auth.
10. **Commit all work** with descriptive messages and push to the designated branch.
11. **EIP-712 signing** — the prototype implements raw EIP-712 signing without `go-order-utils`. When building the production venue implementation, evaluate whether to use `github.com/Polymarket/go-order-utils` or keep the current raw implementation.

---

## Planned Tech Stack

| Tool | Purpose |
|------|---------|
| `gin` / `chi` | HTTP routing |
| `pgx` + `sqlc` | PostgreSQL access with type-safe generated queries |
| `abigen` | Go bindings for Kuru Solidity contracts (no Go SDK exists) |
| `go-order-utils` (Polymarket) | Polymarket EIP-712 order signing (evaluate vs. current raw impl) |
| Privy SDK / JWT libraries | Auth token verification |
| WebSocket client | Real-time Polymarket CLOB price updates |
| Gnosis Safe SDK | Per-user Polygon proxy wallet deployment |

---

## Historical Context

| Date | Event |
|------|-------|
| Aug 2023 | Initial Conway's Game of Life (TypeScript + HTML5) |
| Mar 2026 | Repository pivoted to prediction markets backend |
| Mar 2026 | All frontend files removed; `ARCHITECTURE.md` added |
| Mar 2026 | `CLAUDE.md` created; UX review completed (`suggested-changes.md`) |
| Mar 2026 | Polymarket latency prototype implemented (`PLAN.md` + code) |
| Mar 2026 | Trading UX & bridging options analysis (`trading-ux-options.md`) |
