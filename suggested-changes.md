# Suggested UX Changes

UX review of `ARCHITECTURE.md` focused on speed, latency, and user experience.
The following changes were agreed upon. Bridge latency was raised but skipped —
intent-based bridges (e.g. Across) resolve in 4–5s, which is acceptable.

---

## 1. Real-Time Order Status via WebSocket / SSE

**Problem**: The current design has mobile polling `GET /orders/:id` for status
updates. This means status changes are delayed by the polling interval, creates
unnecessary backend load, and produces a poor experience on slow fills.

**Change**: Add a streaming endpoint for order status:

```
GET /api/v1/orders/:id/stream   — SSE or WebSocket
```

When an order transitions state (`bridging → submitted → filled / failed`), the
backend pushes the update immediately. The existing persistent Polymarket
WebSocket connection is a natural model to follow.

---

## 2. Eager Gnosis Safe Provisioning

**Problem**: The Gnosis Safe on Polygon is currently deployed lazily — on the
user's first Polymarket trade. This introduces a hidden multi-minute delay the
first time a user tries to buy a Polymarket market, with no clear explanation.

**Change**: Deploy the Gnosis Safe during user onboarding / first authentication,
not on first trade.

- Show a one-time "Setting up your cross-chain account…" progress state at login
- Use the RelayClient for gasless deployment (zero cost to user)
- Store the resulting address in `users.polymarket_proxy_address` as before
- By the time a user places their first Polymarket trade, the Safe is already ready

---

## 3. Trigger-Based Position Sync After Trade Fill

**Problem**: The position sync worker runs every 60s. After a user's own trade
fills, their portfolio won't reflect the new position for up to a minute, making
it look like the trade didn't work.

**Change**: When an order transitions to `filled`, immediately trigger a targeted
position sync for the affected user and market — don't wait for the next 60s
tick. The background worker cadence stays the same for general reconciliation.

---

## 4. Single Unified Balance (Aggregate Total)

**Problem**: Users have USDC split across Monad wallet, Polygon Gnosis Safe, and
potentially in-transit during bridge operations. Surfacing multiple balances is
confusing — users should see one number.

**Change**: The `GET /api/v1/user/balances` endpoint returns a single aggregated
USDC balance:

```
total_balance   = monad_balance + polygon_balance + in_transit_amount
```

The backend tracks the breakdown internally (useful for routing trades and
monitoring bridge operations) but the API surface exposes only the unified total.
An optional `breakdown` field can be included for debugging / support purposes
but should not be surfaced in the main UI.

---

## 5. On-Demand Market Price Refresh + WebSocket Subscription per Viewed Market

**Problem**: Polymarket markets sync every 2–5 minutes. For active markets
(live events, imminent resolution), prices can move significantly in that window.
A user trading on a stale price is a trust-eroding experience.

**Changes**:

- Add `POST /api/v1/markets/:id/refresh` — forces a live price fetch from the
  venue when a user opens a market detail view
- Use the existing Polymarket WebSocket (`wss://ws-subscriptions-clob.polymarket.com`)
  to subscribe to price updates for markets that are actively being viewed, not
  just markets where the user holds positions. Subscribe on open, unsubscribe on
  close/navigate-away (tracked via client heartbeat or explicit unsubscribe call)

---

## 6. PostgreSQL Full-Text Search on Markets

**Problem**: `GET /api/v1/markets/search?q=` against thousands of Polymarket
markets without a text index will be slow and will degrade as market count grows.

**Change**: Add a `tsvector` index to the `markets` table on `question` and
`description` at schema creation time:

```sql
ALTER TABLE markets ADD COLUMN search_vector tsvector
    GENERATED ALWAYS AS (
        to_tsvector('english', coalesce(question, '') || ' ' || coalesce(description, ''))
    ) STORED;

CREATE INDEX markets_search_idx ON markets USING GIN (search_vector);
```

The search endpoint uses `search_vector @@ plainto_tsquery(...)` instead of
`ILIKE`.

---

## 7. Cached Balances with Async Refresh

**Problem**: `GET /api/v1/user/balances` could hit live chain RPC calls,
adding 1–3s latency to every portfolio or balance view.

**Change**:

- Cache Monad and Polygon balances in the DB (updated by the existing Position
  Sync worker, or a dedicated lightweight balance poller)
- The endpoint returns the cached value immediately, plus a `last_updated_at`
  timestamp
- On each call, trigger a background async refresh so the next read is fresh
- The client can display "Updated Xs ago" if desired, or simply rely on the
  near-real-time cache being fresh enough

---

## Summary

| # | Change | Status |
|---|--------|--------|
| 1 | Real-time order status (WebSocket/SSE) | Agreed |
| 2 | Eager Gnosis Safe provisioning at login | Agreed |
| 3 | Trigger-based position sync on fill | Agreed |
| 4 | Single unified balance (aggregate total) | Agreed |
| 5 | On-demand market refresh + per-market WebSocket | Agreed |
| 6 | PostgreSQL full-text search index | Agreed |
| 7 | Cached balances with async refresh | Agreed |
