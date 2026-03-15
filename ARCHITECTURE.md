# Prediction Market Backend — High-Level Architecture Plan

## Context

We're building a Go backend for a mobile app that lets users trade prediction markets across **Kuru** (Monad, 2-3 markets) and **Polymarket** (many markets) through a single unified interface. The user's entry point is a **Privy wallet holding USDC on Monad**. The user should never know which platform a market lives on.

**Key challenge**: Two different trading venues with different chains, different settlement, and different APIs — presented as one seamless experience.

---

## 1. Service Architecture

Single Go monolith (for MVP speed), organized into clean internal packages:

```
cmd/server/           — main entrypoint
internal/
  api/                — HTTP handlers (gin/chi router)
  auth/               — Privy JWT verification middleware
  market/             — Unified market service (aggregation, search, caching)
  trade/              — Trade orchestration (routing, execution)
  portfolio/          — Positions, PnL, activity feed
  bridge/             — Bridge abstraction interface + provider impls
  venue/              — Venue interface + implementations
    venue/kuru/       — Kuru order book client
    venue/polymarket/ — Polymarket CLOB client + proxy wallet mgmt
  user/               — User management, wallet associations
  sync/               — Background workers for market/position syncing
  db/                 — Database access layer (sqlc or pgx)
pkg/
  models/             — Shared domain types
```

### Key Abstractions

```go
// Venue interface — each platform implements this
type Venue interface {
    ID() string
    ListMarkets(ctx) ([]Market, error)
    GetMarket(ctx, id) (*Market, error)
    PlaceOrder(ctx, order) (*OrderResult, error)
    GetPositions(ctx, userID) ([]Position, error)
    GetOrderHistory(ctx, userID) ([]Order, error)
}

// Bridge interface — swappable provider
type Bridge interface {
    EstimateFee(ctx, amount) (*BridgeFee, error)
    InitiateTransfer(ctx, from, to, amount) (*BridgeTx, error)
    GetTransferStatus(ctx, txID) (*BridgeStatus, error)
}
```

---

## 2. Authentication

**Recommendation: Privy Auth → Backend JWT**

1. User authenticates via Privy SDK on mobile (wallet-based)
2. Mobile sends Privy auth token to backend
3. Backend verifies token with Privy's JWKS endpoint
4. Backend issues its own short-lived JWT (contains userID, wallet address)
5. All subsequent API calls use this JWT

This gives us control over session management while keeping Privy as the identity source.

---

## 3. Database Schema (Key Tables)

```sql
-- Users & wallets
users (
    id UUID PK,
    privy_user_id TEXT UNIQUE,
    monad_wallet_address TEXT,            -- Privy wallet on Monad
    polymarket_proxy_address TEXT,         -- Gnosis Safe on Polygon (created on first Polymarket trade)
    polymarket_api_key_enc TEXT,           -- encrypted Polymarket CLOB API creds
    polymarket_api_secret_enc TEXT,
    polymarket_passphrase_enc TEXT,
    created_at, updated_at
)

-- Normalized market data from both venues
markets (
    id UUID PK,
    venue TEXT,                        -- 'kuru' | 'polymarket'
    venue_market_id TEXT,              -- ID on the source platform
    question TEXT,
    description TEXT,
    category TEXT,
    outcome_yes_token_id TEXT,
    outcome_no_token_id TEXT,
    yes_price DECIMAL,
    no_price DECIMAL,
    volume_24h DECIMAL,
    liquidity DECIMAL,
    resolution_date TIMESTAMPTZ,
    status TEXT,                       -- 'active' | 'resolved' | 'closed'
    resolved_outcome TEXT,
    image_url TEXT,
    metadata JSONB,                    -- venue-specific extras
    last_synced_at TIMESTAMPTZ,
    created_at, updated_at,
    UNIQUE(venue, venue_market_id)
)

-- All orders placed through our system
orders (
    id UUID PK,
    user_id UUID FK,
    market_id UUID FK,
    venue TEXT,
    side TEXT,                         -- 'buy' | 'sell'
    outcome TEXT,                      -- 'yes' | 'no'
    amount DECIMAL,                    -- USDC amount
    price DECIMAL,                     -- price per share
    shares DECIMAL,                    -- shares received
    status TEXT,                       -- 'pending' | 'bridging' | 'submitted' | 'filled' | 'partial' | 'failed' | 'cancelled'
    bridge_tx_id TEXT,                 -- if polymarket, the bridge transaction
    venue_order_id TEXT,               -- order ID on the venue
    error_message TEXT,
    created_at, updated_at
)

-- Current positions (synced from venues + computed from orders)
positions (
    id UUID PK,
    user_id UUID FK,
    market_id UUID FK,
    venue TEXT,
    outcome TEXT,
    shares DECIMAL,
    avg_entry_price DECIMAL,
    current_price DECIMAL,
    unrealized_pnl DECIMAL,
    realized_pnl DECIMAL,
    last_synced_at TIMESTAMPTZ,
    created_at, updated_at,
    UNIQUE(user_id, market_id, outcome)
)

-- Bridge transactions
bridge_transactions (
    id UUID PK,
    user_id UUID FK,
    order_id UUID FK,
    provider TEXT,                     -- 'across' | 'debridge' | 'relay'
    direction TEXT,                    -- 'monad_to_polygon' | 'polygon_to_monad'
    amount DECIMAL,
    fee DECIMAL,
    status TEXT,                       -- 'initiated' | 'pending' | 'completed' | 'failed'
    source_tx_hash TEXT,
    dest_tx_hash TEXT,
    created_at, updated_at
)

-- Activity feed (denormalized for fast queries)
activity (
    id UUID PK,
    user_id UUID FK,
    type TEXT,                         -- 'trade' | 'bridge' | 'deposit' | 'withdrawal' | 'resolution'
    market_id UUID FK,
    order_id UUID FK,
    title TEXT,
    description TEXT,
    amount DECIMAL,
    metadata JSONB,
    created_at
)
```

---

## 4. API Endpoints

### Auth
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/auth/login` | Verify Privy token, return JWT |
| POST | `/api/v1/auth/refresh` | Refresh JWT |

### Markets
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/markets` | List all markets (paginated, filterable by category/status, sorted by volume/trending) |
| GET | `/api/v1/markets/:id` | Market detail (includes order book depth for Kuru, price history) |
| GET | `/api/v1/markets/:id/prices` | Price history / chart data |
| GET | `/api/v1/markets/featured` | Curated/trending markets |
| GET | `/api/v1/markets/search?q=` | Search markets |

### Trading
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/orders` | Place an order (backend routes to correct venue) |
| GET | `/api/v1/orders/:id` | Order status (includes bridge status if applicable) |
| DELETE | `/api/v1/orders/:id` | Cancel pending order |
| GET | `/api/v1/orders` | User's order history (paginated, filterable) |

### Portfolio
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/portfolio` | All current positions with PnL summary |
| GET | `/api/v1/portfolio/pnl` | Aggregated PnL (total, today, 7d, 30d, all-time) |
| GET | `/api/v1/portfolio/history` | Historical portfolio value over time |

### Activity
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/activity` | Combined activity feed (trades, bridges, resolutions) |

### User
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/user/profile` | User profile + wallet info |
| GET | `/api/v1/user/balances` | USDC balance on Monad + Polygon proxy wallet |

---

## 5. Key Data Flows

### (a) Browsing Markets
```
Mobile → GET /markets → Market Service
    → Query `markets` table (already synced)
    → Return unified list with current prices

Background: Market Sync Worker
    → Kuru (every 30s): Query on-chain order book state for 2-3 markets
    → Polymarket (every 2-5min): Sync all active markets via Polymarket API
        → Full sync on startup, incremental updates after
        → Prices updated more frequently for markets users hold positions in
    → Upsert into `markets` table
```

### (b) Placing a Kuru Trade (Client-Side Signing — MVP)
```
Mobile → POST /orders {market_id, side, outcome, amount}
    → Trade Service detects venue = "kuru"
    → Create order record (status: "pending")
    → Build unsigned tx (calldata, gas estimate, contract address)
    → Return tx params to mobile

Mobile signs via Privy SDK → submits to Monad
    → POST /orders/:id/confirm {tx_hash}
    → Backend monitors tx confirmation
    → Update order status → "filled" / "failed"
    → Update positions table
    → Insert activity record
```

**Future: Server-Side Signing for Advanced Orders**
For limit orders, stop-losses, etc. the backend will need Privy server-side wallet signing. The order model already supports this — the `status` flow stays the same, but execution shifts to the backend. We account for this by keeping the Venue interface execution-agnostic (it returns either tx params for client signing OR executes directly).

### (c) Placing a Polymarket Trade
```
Mobile → POST /orders {market_id, side, outcome, amount}
    → Trade Service detects venue = "polymarket"
    → Ensure user has Gnosis Safe deployed on Polygon (deploy on first trade)
    → Create order record (status: "bridging")
    → Bridge Service: initiate USDC transfer Monad → Polygon
        → User signs bridge tx on mobile via Privy
        → Bridge Monitor polls for completion
    → On bridge completion:
        → USDC.e arrives in user's Gnosis Safe on Polygon
        → Update order status → "submitted"
        → Polymarket Service: place order via CLOB API
            → Uses Safe as funder, EIP-712 signing
            → Handles token approvals via RelayClient if needed
    → Update order status on fill
    → Update positions table
    → Insert activity record
```

**Gnosis Safe per user**: Each user gets a Polymarket-compatible Safe deployed on Polygon on their first Polymarket trade. The Safe address is stored in `users.polymarket_proxy_address`. We use Polymarket's `safe-wallet-integration` SDK for deployment and the RelayClient for gasless operations.

### (d) Viewing Portfolio / PnL
```
Mobile → GET /portfolio
    → Portfolio Service queries `positions` table
    → Enriches with current prices from `markets` table
    → Computes unrealized PnL = (current_price - avg_entry_price) * shares
    → Returns combined view

Background: Position Sync Worker runs every 60s
    → Kuru: Check on-chain balances for outcome tokens
    → Polymarket: Query CLOB API for user positions
    → Reconcile with `positions` table
```

---

## 6. Background Workers

| Worker | Frequency | Purpose |
|--------|-----------|---------|
| Market Sync | 30s | Sync market data + prices from both venues |
| Position Sync | 60s | Reconcile positions with on-chain / CLOB state |
| Bridge Monitor | 10s | Poll bridge status for pending transfers |
| PnL Snapshot | 5min | Snapshot portfolio value for historical charts |
| Market Resolution | 5min | Check for resolved markets, auto-redeem winning shares (server-side signing), auto-bridge proceeds back to Monad |
| Polymarket WebSocket | persistent | Maintain WebSocket connection to Polymarket CLOB for real-time price updates |
| Auto-Bridge Back | on-event | After Polymarket sell fills or resolution redemption, auto-bridge USDC back to Monad |

---

## 7. Bridge Abstraction

```go
type BridgeProvider interface {
    Name() string
    EstimateFee(ctx, req EstimateRequest) (*FeeEstimate, error)
    Initiate(ctx, req TransferRequest) (*TransferResult, error)
    Status(ctx, transferID string) (*TransferStatus, error)
}

// Start with one provider (e.g., Across or deBridge), add more later
// Provider selected at config level, not per-request
```

---

## 8. Decided & Remaining Questions

**Decided:**
- Trade signing: Client-side via Privy SDK (MVP), with server-side signing planned for advanced order types
- Proxy wallet: Gnosis Safe per user on Polygon, using Polymarket's Safe integration SDK
- Market scope: All active Polymarket markets synced

- Withdrawal flow: Auto-bridge USDC back to Monad after selling Polymarket positions. User's funds always consolidate back to Monad.
- Price updates: WebSocket connection to Polymarket CLOB (`wss://ws-subscriptions-clob.polymarket.com`) for real-time price updates. Polling as fallback.
- Market resolution: Auto-redeem winning shares via server-side signing, then auto-bridge proceeds back to Monad. This is a key use case for server-side Privy wallet signing.

---

## 9. Implementation Notes

**Kuru (no Go SDK)**: Kuru only has a TypeScript SDK. Use `abigen` to generate Go contract bindings from Kuru contract ABIs and interact with the order book contracts directly via `go-ethereum`. The contracts are the stable interface.

**Polymarket Go tooling**: Use Polymarket's `go-order-utils` (github.com/Polymarket/go-order-utils) for EIP-712 order signing. Build a thin REST client against the CLOB API for order placement. Use the Gamma API for market metadata sync.

**Polymarket CLOB auth**: Each user needs CLOB API credentials (key, secret, passphrase) derived from an L1 auth signature (EIP-712). These are created on first Polymarket trade and stored encrypted in the `users` table.

**Async Polymarket trades**: The Polymarket trade flow (bridge + CLOB) is async. The `POST /orders` endpoint returns immediately with `status: "bridging"`. The mobile app polls `GET /orders/:id` for updates. Background goroutines handle bridge monitoring and CLOB submission. No job queue needed at MVP — goroutine + DB status tracking is sufficient.

**Auto-bridge back to Monad**: When a user sells a Polymarket position or a market resolves, the backend automatically bridges USDC back from Polygon to Monad. The sell flow becomes: CLOB sell → fill confirmed → initiate bridge Polygon→Monad → update user's Monad balance. This keeps all idle funds on Monad.

**Auto-redemption on resolution**: The Market Resolution worker detects resolved markets where the user holds winning shares. Using server-side Privy signing, it: (1) redeems conditional tokens on Polymarket, (2) bridges the USDC proceeds back to Monad, (3) updates positions and creates activity records. This is a key driver for server-side signing — the user doesn't need to manually claim.

---

## 10. Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Bridge failure mid-trade | Track bridge status in DB; funds stay on source chain if bridge fails; surface clear error to user |
| No Go SDK for Kuru | Generate Go bindings from ABIs via abigen; contracts are the stable interface |
| Polymarket proxy wallet complexity | Lazy setup on first trade; store encrypted creds in DB |
| Price staleness | Background refresh every 15-30s for positions; on-demand fetch for market detail |
| Signing key security | Use KMS (AWS/GCP) in production for any server-side signing; encrypted env var for MVP |

---

## 11. Verification / Testing Plan

1. **Unit tests**: Each venue implementation against mocked responses
2. **Integration tests**: Against Polymarket testnet CLOB + Monad testnet
3. **Manual E2E**:
   - Create user → list markets → place Kuru trade → verify position shows up
   - Create user → place Polymarket trade → verify bridge + CLOB execution → verify combined portfolio
4. **Load test**: Market sync workers under load to ensure DB doesn't bottleneck
