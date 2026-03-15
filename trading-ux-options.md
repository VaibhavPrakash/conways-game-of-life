# Unified Market Trading UX — Bridging & Execution Options

Analysis of options for providing a seamless trading experience where users
don't know the difference between a Polymarket market and a Kuru market.

The core challenge: Kuru trades execute directly on Monad (~1s). Polymarket
trades require bridging USDC from Monad to Polygon before placing a CLOB order.
The options below address how to handle (or eliminate) that bridge latency.

---

## Option A: Bridge-per-Trade

Every Polymarket trade bridges USDC from Monad → Polygon in real-time.

```
User taps "Buy" on Polymarket market
  → Backend bridges USDC Monad→Polygon (~5s)
  → Places CLOB order on Polymarket
  → Total: ~6-10s
```

- **Speed: ~6-10s**
- **Pros:**
  - Simplest to implement
  - No pre-funding or platform capital needed
  - Funds always consolidated on Monad
  - Self-custodial — user owns everything
- **Cons:**
  - Slowest option — user waits for bridge every trade
  - Bridge fees on every trade
  - Feels noticeably different from Kuru (~1s)

---

## Option B: Pre-funded Polygon Float

Keep USDC.e in the user's Gnosis Safe on Polygon. Rebalance in the background.

```
User taps "Buy" on Polymarket market
  → Safe already has USDC.e → place order immediately (~0.5-1s)
  → Background: bridge more USDC from Monad to replenish

If Safe is empty → falls back to bridge-per-trade (Option A)
```

- **Speed: ~0.5-1s** (when funds present), **~6-10s** (fallback to bridge)
- **Pros:**
  - Matches Kuru speed when funded
  - Still self-custodial
  - Simple concept — just bridge extra ahead of time
- **Cons:**
  - User capital sits idle on Polygon
  - Requires per-user Gnosis Safe + Polymarket API creds
  - First trade still slow (initial bridge)
  - Need a rebalancing strategy (threshold-based, predictive, or user-triggered)

---

## Option C: Platform Liquidity Pool

Platform maintains a shared USDC.e pool on Polygon. Advances funds to the
user's Gnosis Safe instantly, then bridges the user's Monad USDC to repay
the pool asynchronously.

```
User taps "Buy" on Polymarket market
  → Platform transfers USDC.e from pool → user's Safe (~1-1.5s)
  → Places CLOB order
  → Background: bridge user's Monad USDC to repay pool

If pool is empty → falls back to bridge-per-trade (Option A)
```

- **Speed: ~1.5-4s** (pool → Safe transfer), **~0.5-1s** (if Safe already approved)
- **Pros:**
  - Fast without pre-funding every user
  - Capital efficient — one pool serves all users
  - Still self-custodial (user's Safe holds the position)
- **Cons:**
  - Platform needs capital for the pool
  - Bridge risk — what if repayment bridge fails?
  - Still need per-user Gnosis Safes + Polymarket API creds
  - More complex than A or B

---

## Option D: Hybrid Smart Routing (B + C + A fallback)

Try the user's existing Polygon balance first, then pool advance, then bridge
as last resort.

```go
if user.PolygonBalance >= order.Amount {
    // User has funds on Polygon → instant (~0.5-1s)
} else if pool.Available >= order.Amount {
    // Advance from platform pool (~1.5-4s)
} else {
    // Fallback: bridge then trade (~6-10s)
}
```

- **Speed: ~0.5-1s** (best case), **~1.5-4s** (pool), **~6-10s** (worst case)
- **Pros:**
  - Best possible speed in every scenario
  - Graceful degradation
  - Still self-custodial
- **Cons:**
  - Most complex — three code paths to build, test, and monitor
  - Still need per-user Safes, creds, approvals
  - Platform capital needed for pool layer

---

## Option E: Platform-as-Broker

Platform takes USDC on Monad and places trades on Polymarket from its own
account. Users never touch Polygon. An internal ledger tracks who owns what.

```
User taps "Buy" on Polymarket market
  → User sends USDC to platform on Monad (~0.5-1s)
  → Platform places order from its own Polymarket account (~0.3-0.5s)
  → Internal ledger records: "User X owns N shares of market Y"
  → Total: ~1s
```

- **Speed: ~1s**
- **Pros:**
  - Fastest consistent speed — no bridge in user flow at all
  - No per-user Gnosis Safes, no per-user API creds, no per-user approvals
  - Simpler infrastructure — one platform Polymarket account
  - Internal netting possible (User A buys Yes, User B buys No → no Polymarket
    order needed, platform keeps both sides)
  - Operationally simpler at scale
- **Cons:**
  - You custody user funds — legal/regulatory burden
  - Platform is a single point of failure
  - Need rock-solid internal ledger and accounting
  - Platform needs capital on Polygon
  - Users can't independently recover positions if platform goes down

---

## Quick Comparison

| Option | Speed | Self-custodial? | Platform capital? | Infra complexity |
|--------|-------|----------------|-------------------|-----------------|
| **A** Bridge-per-trade | 6-10s | Yes | No | Low |
| **B** Pre-funded float | 0.5-1s* | Yes | No | Medium |
| **C** Platform pool | 1.5-4s | Yes | Yes | Medium-High |
| **D** Hybrid routing | 0.5-1s* | Yes | Yes | High |
| **E** Platform-as-broker | ~1s | No | Yes | Medium |

*\*When funds are already on Polygon; falls back to slower paths otherwise.*

---

## Bridge Provider Options

All options above (except E, which only bridges for platform rebalancing) need
a bridge provider. Current implementation uses Relay.

| Provider | Type | Speed | Notes |
|----------|------|-------|-------|
| **Relay** (current) | Intent-based | ~4-5s | Already implemented in `internal/relay/`. Simple REST API |
| **Across** | Intent-based | ~2-6s | Mature, good Polygon support. Good fallback option |
| **deBridge** | Intent-based | ~3-5s | Has a Go SDK. Good for programmatic use |
| **Socket/Bungee** | Aggregator | Varies | Meta-bridge, picks best route. Adds route computation latency |

**Recommendation:** Keep Relay as primary, add Across as fallback behind the
`BridgeProvider` interface defined in `ARCHITECTURE.md`.

---

## Recommendation

For **MVP**: Option B (pre-funded float) with fallback to Option A
(bridge-per-trade). Bridge more than the trade amount on first trade so
subsequent trades are instant.

For **V2**: Evaluate Option E (platform-as-broker) if volume justifies the
custody/regulatory overhead. The internal netting alone can save significant
costs at scale.
