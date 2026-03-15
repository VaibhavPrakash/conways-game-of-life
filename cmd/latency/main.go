package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/VaibhavPrakash/conways-game-of-life/internal/polymarket"
	"github.com/VaibhavPrakash/conways-game-of-life/internal/relay"
	"github.com/VaibhavPrakash/conways-game-of-life/internal/timing"
	"github.com/VaibhavPrakash/conways-game-of-life/internal/wallet"
)

func main() {
	// --- CLI flags ---
	privateKeyFlag := flag.String("private-key", "", "Hex-encoded private key (or set PRIVATE_KEY env var)")
	marketIDFlag := flag.String("market-id", "", "Polymarket token ID to trade (required)")
	amountFlag := flag.Float64("amount", 1.0, "USDC amount to trade")
	sideFlag := flag.String("side", "buy", `Order side: "buy" or "sell"`)
	priceFlag := flag.Float64("price", 0.50, "Limit price (e.g. 0.50)")
	runsFlag := flag.Int("runs", 1, "Number of iterations")
	skipBridgeFlag := flag.Bool("skip-bridge", false, "Skip bridge step if USDC.e already on Polygon")
	dryRunFlag := flag.Bool("dry-run", false, "Go through all steps but do not submit the final CLOB order")
	relayAPIKeyFlag := flag.String("relay-api-key", "", "Optional Relay API key (or set RELAY_API_KEY env var)")
	flag.Parse()

	// Resolve private key from flag or env.
	privateKeyHex := *privateKeyFlag
	if privateKeyHex == "" {
		privateKeyHex = os.Getenv("PRIVATE_KEY")
	}
	if privateKeyHex == "" {
		log.Fatalf("error: --private-key flag or PRIVATE_KEY environment variable is required")
	}

	// Resolve Relay API key from flag or env.
	relayAPIKey := *relayAPIKeyFlag
	if relayAPIKey == "" {
		relayAPIKey = os.Getenv("RELAY_API_KEY")
	}

	if *marketIDFlag == "" {
		log.Fatalf("error: --market-id is required")
	}

	if *priceFlag <= 0 || *priceFlag >= 1 {
		log.Fatalf("error: --price must be between 0 (exclusive) and 1 (exclusive), got %f", *priceFlag)
	}

	if *amountFlag <= 0 {
		log.Fatalf("error: --amount must be positive, got %f", *amountFlag)
	}

	// Parse side.
	var orderSide polymarket.OrderSide
	switch strings.ToLower(*sideFlag) {
	case "buy":
		orderSide = polymarket.Buy
	case "sell":
		orderSide = polymarket.Sell
	default:
		log.Fatalf("error: --side must be \"buy\" or \"sell\", got %q", *sideFlag)
	}

	// Derived values.
	amount := *amountFlag
	price := *priceFlag
	size := amount / price // e.g. $1 at $0.50 = 2 shares

	// Print startup header.
	fmt.Println("========================================")
	fmt.Println("  Polymarket Latency Testing Prototype")
	fmt.Println("========================================")
	fmt.Printf("  Market ID   : %s\n", *marketIDFlag)
	fmt.Printf("  Amount      : %.6f USDC\n", amount)
	fmt.Printf("  Side        : %s\n", strings.ToUpper(*sideFlag))
	fmt.Printf("  Price       : %.4f\n", price)
	fmt.Printf("  Size        : %.4f shares\n", size)
	fmt.Printf("  Runs        : %d\n", *runsFlag)
	fmt.Printf("  Skip bridge : %v\n", *skipBridgeFlag)
	fmt.Printf("  Dry run     : %v\n", *dryRunFlag)
	fmt.Println("========================================")

	ctx := context.Background()

	// Load wallet once — reused across all runs.
	w, err := wallet.FromPrivateKey(privateKeyHex)
	if err != nil {
		log.Fatalf("load wallet: %v", err)
	}
	defer w.Close()
	fmt.Printf("\nWallet address: %s\n", w.Address.Hex())

	// Relay client (stateless, reuse across runs).
	relayClient := relay.NewClient(relayAPIKey)

	// Raw USDC amount for the bridge quote (6 decimals).
	rawAmount := wallet.ParseUSDC(amount)

	// Polymarket API credentials — derived once and cached across runs.
	var cachedCreds *polymarket.APICredentials

	summary := timing.NewSummary()

	for run := 1; run <= *runsFlag; run++ {
		fmt.Printf("\n--- Run %d/%d ---\n", run, *runsFlag)

		tracker := timing.New()

		// Print current balances.
		monadBal, err := w.BalanceOf(ctx, wallet.MonadRPC, wallet.USDCMonad)
		if err != nil {
			log.Fatalf("run %d: get Monad USDC balance: %v", run, err)
		}
		polygonBal, err := w.BalanceOf(ctx, wallet.PolygonRPC, wallet.USDCePolygon)
		if err != nil {
			log.Fatalf("run %d: get Polygon USDC.e balance: %v", run, err)
		}
		fmt.Printf("  Balances — Monad USDC: %s  |  Polygon USDC.e: %s\n",
			wallet.FormatUSDC(monadBal), wallet.FormatUSDC(polygonBal))

		// ----------------------------------------------------------------
		// Step 1: Bridge (Relay) — Monad USDC → Polygon USDC.e
		// ----------------------------------------------------------------
		if !*skipBridgeFlag {
			// 1a. Get quote.
			tracker.Start("Relay: get quote")
			quoteResp, err := relayClient.GetQuote(ctx, relay.QuoteRequest{
				User:                w.Address.Hex(),
				OriginChainID:       wallet.MonadChainID,
				DestinationChainID:  wallet.PolygonChainID,
				OriginCurrency:      wallet.USDCMonad,
				DestinationCurrency: wallet.USDCePolygon,
				Amount:              rawAmount.String(),
				TradeType:           "EXACT_INPUT",
			})
			tracker.Stop()
			if err != nil {
				log.Fatalf("run %d: relay get quote: %v", run, err)
			}
			if len(quoteResp.Steps) == 0 {
				log.Fatalf("run %d: relay quote returned no steps", run)
			}

			// 1b. Extract tx data from the first step and submit the bridge tx.
			firstStep := quoteResp.Steps[0]
			txData, requestID, err := relay.GetStepTxData(firstStep)
			if err != nil {
				log.Fatalf("run %d: extract relay step tx data: %v", run, err)
			}

			// Parse Value (hex string, e.g. "0x1234") → *big.Int.
			valueStr := strings.TrimPrefix(txData.Value, "0x")
			var txValue *big.Int
			if valueStr == "" || valueStr == "0" {
				txValue = big.NewInt(0)
			} else {
				txValue = new(big.Int)
				_, ok := txValue.SetString(valueStr, 16)
				if !ok {
					log.Fatalf("run %d: parse relay tx value %q: invalid hex", run, txData.Value)
				}
			}

			// Decode Data (0x-prefixed hex) → []byte.
			txCalldata := common.FromHex(txData.Data)

			// Parse destination address.
			toAddr := relay.ParseTxDataTo(txData)

			tracker.Start("Relay: submit bridge tx")
			bridgeTxHash, err := w.SendTx(ctx, wallet.MonadRPC, wallet.MonadChainID, toAddr, txCalldata, txValue)
			tracker.Stop()
			if err != nil {
				log.Fatalf("run %d: submit bridge tx: %v", run, err)
			}
			fmt.Printf("  Bridge tx submitted: %s\n", bridgeTxHash.Hex())

			// 1c. Poll Relay until bridge completes.
			tracker.Start("Relay: bridge complete")
			_, err = relayClient.PollStatus(ctx, requestID, 3*time.Second, 10*time.Minute)
			tracker.Stop()
			if err != nil {
				log.Fatalf("run %d: bridge polling failed: %v", run, err)
			}

			// Print updated Polygon balance after bridge.
			polygonBalAfter, err := w.BalanceOf(ctx, wallet.PolygonRPC, wallet.USDCePolygon)
			if err != nil {
				log.Fatalf("run %d: get Polygon USDC.e balance after bridge: %v", run, err)
			}
			fmt.Printf("  Polygon USDC.e after bridge: %s\n", wallet.FormatUSDC(polygonBalAfter))
		}

		// ----------------------------------------------------------------
		// Step 2: Derive Polymarket API credentials (cached after first run).
		// ----------------------------------------------------------------
		if cachedCreds == nil {
			tracker.Start("CLOB: derive API key")
			creds, err := polymarket.DeriveAPICredentials(ctx, w.PrivateKey, w.Address)
			tracker.Stop()
			if err != nil {
				log.Fatalf("run %d: derive polymarket API credentials: %v", run, err)
			}
			cachedCreds = creds
			fmt.Printf("  Polymarket API key derived: %s\n", cachedCreds.APIKey)
		}

		// ----------------------------------------------------------------
		// Step 3: Ensure USDC.e allowance for CTFExchange on Polygon.
		// ----------------------------------------------------------------
		needed := wallet.ParseUSDC(amount)
		allowance, err := w.Allowance(ctx, wallet.PolygonRPC, wallet.USDCePolygon, polymarket.CTFExchange)
		if err != nil {
			log.Fatalf("run %d: check USDC.e allowance: %v", run, err)
		}
		if allowance.Cmp(needed) < 0 {
			fmt.Printf("  USDC.e allowance (%s) < needed (%s), approving max...\n",
				wallet.FormatUSDC(allowance), wallet.FormatUSDC(needed))

			// max uint256
			maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))

			tracker.Start("USDC.e approval")
			approveTxHash, err := w.Approve(ctx, wallet.PolygonRPC, wallet.PolygonChainID,
				wallet.USDCePolygon, polymarket.CTFExchange, maxUint256)
			tracker.Stop()
			if err != nil {
				log.Fatalf("run %d: approve USDC.e: %v", run, err)
			}
			fmt.Printf("  Approval tx: %s — waiting for receipt...\n", approveTxHash.Hex())

			approvalCtx, approvalCancel := context.WithTimeout(ctx, 5*time.Minute)
			_, err = w.WaitForTx(approvalCtx, wallet.PolygonRPC, approveTxHash)
			approvalCancel()
			if err != nil {
				log.Fatalf("run %d: wait for approval tx: %v", run, err)
			}
			fmt.Println("  Approval confirmed.")
		}

		// ----------------------------------------------------------------
		// Step 4: Build and sign CLOB order.
		// ----------------------------------------------------------------
		pmClient := polymarket.NewClient(cachedCreds, w.PrivateKey, w.Address)

		orderReq := polymarket.OrderRequest{
			TokenID: *marketIDFlag,
			Price:   price,
			Size:    size,
			Side:    orderSide,
		}

		tracker.Start("CLOB: build+sign order")
		signedOrder, err := pmClient.BuildAndSignOrder(orderReq)
		tracker.Stop()
		if err != nil {
			log.Fatalf("run %d: build and sign order: %v", run, err)
		}

		// ----------------------------------------------------------------
		// Step 5: Submit order and poll for fill (unless --dry-run).
		// ----------------------------------------------------------------
		if !*dryRunFlag {
			tracker.Start("CLOB: submit order")
			orderResp, err := pmClient.SubmitOrder(ctx, signedOrder, "GTC")
			tracker.Stop()
			if err != nil {
				log.Fatalf("run %d: submit order: %v", run, err)
			}
			fmt.Printf("  Order submitted: ID=%s  status=%s\n", orderResp.OrderID, orderResp.Status)

			tracker.Start("CLOB: order filled")
			fillStatus, err := pmClient.PollOrderFill(ctx, orderResp.OrderID, 1*time.Second, 2*time.Minute)
			tracker.Stop()
			if err != nil {
				log.Fatalf("run %d: waiting for order fill: %v", run, err)
			}
			fmt.Printf("  Order %s final status: %s\n", fillStatus.ID, fillStatus.Status)
		} else {
			fmt.Println("  [dry-run] Skipping CLOB order submission.")
		}

		// Print per-run timing table.
		tracker.PrintTable(fmt.Sprintf("Run %d timing", run))
		summary.Add(tracker)
	}

	// Print aggregate summary across all runs.
	summary.Print()
}
