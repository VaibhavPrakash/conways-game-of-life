# Polymarket Latency Prototype — Plan

## Goal

A standalone Go CLI tool that measures end-to-end latency of placing an order on Polymarket starting from USDC on Monad. The pipeline: **Monad USDC → Relay bridge → Polygon USDC.e → Polymarket CLOB order**. Each step is timed independently so we can see where latency lives.

---

## Scope

This is a **throwaway prototype** — no database, no HTTP server, no auth middleware, no background workers. Just a CLI that runs the pipeline once (or N times) and prints timing results.

---

## Architecture

```
cmd/latency/main.go          — CLI entrypoint, flags, orchestration
internal/
  relay/relay.go              — Relay bridge client (quote + execute + poll status)
  polymarket/
    auth.go                   — L1/L2 CLOB authentication (API key derivation, HMAC signing)
    client.go                 — CLOB REST client (post order, get order status)
    order.go                  — Order building + EIP-712 signing (wraps go-order-utils)
  wallet/wallet.go            — Ethereum key management, tx signing, tx submission
  timing/timing.go            — Stopwatch helpers for latency measurement
```

---

## Steps (in order of implementation)

### Step 1: Project scaffolding
- `go mod init`
- Create directory structure
- Add dependencies: `go-ethereum`, `go-order-utils`, and standard libs
- Create `cmd/latency/main.go` with CLI flags:
  - `--private-key` (hex, or env `PRIVATE_KEY`)
  - `--market-id` (Polymarket condition ID / token ID to trade)
  - `--amount` (USDC amount, e.g. `1.0`)
  - `--side` (buy/sell, default buy)
  - `--price` (limit price, e.g. `0.50`)
  - `--runs` (number of iterations, default 1)
  - `--skip-bridge` (if USDC.e is already on Polygon, skip the bridge step)
  - `--dry-run` (go through all steps but don't submit the final order)

### Step 2: Wallet management (`internal/wallet/`)
- Load private key from flag/env
- Derive Monad address and Polygon address (same key, different chains)
- Helper: sign and send transaction on a given chain (accepts RPC URL + chain ID)
- Helper: check USDC balance on Monad (`0x754704Bc059F8C67012fEd69BC8A327a5aafb603`)
- Helper: check USDC.e balance on Polygon (`0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174`)
- Helper: approve ERC-20 spending (for exchange contract allowance)

### Step 3: Relay bridge client (`internal/relay/`)
- `GetQuote()` — `POST https://api.relay.link/quote/v2`
  - Origin: Monad (chain 143), USDC `0x754704Bc059F8C67012fEd69BC8A327a5aafb603`
  - Destination: Polygon (chain 137), USDC.e `0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174`
  - Returns transaction steps to sign + submit
- `Execute()` — sign and submit the transaction(s) returned by the quote
- `PollStatus()` — `GET https://api.relay.link/intents/status/v3?requestId=...`
  - Poll every 2s until status is `"success"` or `"failure"`
  - Timeout after 10 minutes
- Timing: measure quote latency, execution latency, bridge completion latency separately

### Step 4: Polymarket CLOB auth (`internal/polymarket/auth.go`)
- **L1 auth**: EIP-712 signature over `ClobAuthDomain` (name, version `"1"`, chainId `137`)
  - Fields: `address`, `timestamp`, `nonce`, `message`
- **Derive API credentials**: `POST https://clob.polymarket.com/auth/derive-api-key`
  - Uses L1 auth headers
  - Returns `apiKey`, `secret`, `passphrase` — cache these for the session
- **L2 auth**: HMAC-SHA256 request signing using the derived credentials
  - Sign: `timestamp + method + path + body` with `secret` as key
  - Headers: `POLY_ADDRESS`, `POLY_SIGNATURE`, `POLY_TIMESTAMP`, `POLY_NONCE`, `POLY_API_KEY`, `POLY_PASSPHRASE`

### Step 5: Polymarket order placement (`internal/polymarket/client.go` + `order.go`)
- **Pre-flight**:
  - Check USDC.e balance on Polygon
  - Ensure USDC.e allowance on CTF Exchange (`0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E`) — approve if needed
- **Build order**:
  - Use `go-order-utils` to construct the EIP-712 typed data
  - Fields: `salt`, `maker`, `signer`, `taker` (zero addr), `tokenId`, `makerAmount`, `takerAmount`, `side`, `expiration`, `nonce`, `feeRateBps`, `signatureType` (0 for EOA)
  - Sign with private key
- **Submit**: `POST https://clob.polymarket.com/order`
  - Include L2 auth headers
  - Body: signed order + `orderType` (GTC)
- **Confirm**: Poll `GET https://clob.polymarket.com/data/order/{orderID}` until filled or timeout
- Timing: measure order build, signing, submission, and fill latency separately

### Step 6: Timing & reporting (`internal/timing/`)
- Simple stopwatch: `Start(label) → Stop(label) → Duration(label)`
- At the end of each run, print a table:

```
Run 1/3:
  ┌────────────────────────┬───────────┐
  │ Step                   │ Latency   │
  ├────────────────────────┼───────────┤
  │ Relay: get quote       │   320ms   │
  │ Relay: submit bridge   │   1.2s    │
  │ Relay: bridge complete │   8m 32s  │
  │ USDC.e approval        │   2.1s    │
  │ CLOB: derive API key   │   480ms   │
  │ CLOB: build+sign order │   12ms    │
  │ CLOB: submit order     │   210ms   │
  │ CLOB: order filled     │   1.8s    │
  ├────────────────────────┼───────────┤
  │ TOTAL                  │   8m 38s  │
  └────────────────────────┴───────────┘
```

- After all runs, print summary (min/max/avg/p50/p95 for each step)

### Step 7: End-to-end orchestration (`cmd/latency/main.go`)
- Wire everything together:
  1. Load wallet
  2. Check Monad USDC balance
  3. If `--skip-bridge` is not set: run Relay bridge flow
  4. Check Polygon USDC.e balance
  5. Ensure USDC.e approval on exchange
  6. Derive Polymarket API credentials (once, reuse across runs)
  7. Build, sign, submit order
  8. Wait for fill
  9. Print timing report
- For `--dry-run`: execute everything except final order submission (still measures bridge + auth latency)
- For multiple `--runs`: loop steps 3-8, reuse API credentials

---

## Key Constants

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

## Dependencies

```
github.com/ethereum/go-ethereum   — Ethereum client, ABI, tx signing
github.com/Polymarket/go-order-utils — EIP-712 order signing for Polymarket
```

No database. No web framework. No config files — just CLI flags and env vars.

---

## What this prototype answers

1. **Bridge latency**: How long does Relay take to move USDC from Monad to Polygon?
2. **CLOB auth latency**: How long to derive API credentials?
3. **Order placement latency**: How long from "order signed" to "order on book"?
4. **Fill latency**: How long from "order on book" to "order filled"?
5. **End-to-end**: Total time from "user clicks buy" to "position acquired"

These numbers inform whether the UX can feel instant (sub-second), near-real-time (seconds), or requires async/polling (minutes).
