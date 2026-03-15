package polymarket

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	CTFExchange        = "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E"
	NegRiskCTFExchange = "0xC5d563A36AE78145C45a50134d48A1215220f80a"
	PolygonChainID     = 137
)

// EIP-712 type strings for CTF Exchange orders.
const (
	orderDomainName    = "CTF Exchange"
	orderDomainVersion = "1"

	// Full type string used for computing the type hash.
	orderTypeString = "Order(uint256 salt,address maker,address signer,address taker,uint256 tokenId,uint256 makerAmount,uint256 takerAmount,uint256 expiration,uint256 nonce,uint256 feeRateBps,uint8 side,uint8 signatureType)"

	// EIP-712 domain type string (no verifyingContract field in chainId-only domains is
	// incorrect — Polymarket's CTF Exchange domain includes verifyingContract).
	orderDomainTypeString = "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"
)

// OrderSide represents the direction of a trade.
type OrderSide int

const (
	Buy  OrderSide = 0
	Sell OrderSide = 1
)

// OrderRequest contains the user-facing parameters for building an order.
type OrderRequest struct {
	TokenID string
	Price   float64 // e.g. 0.50
	Size    float64 // number of shares
	Side    OrderSide
}

// SignedOrder is the fully constructed and signed order ready for submission.
type SignedOrder struct {
	Salt          string `json:"salt"`
	Maker         string `json:"maker"`
	Signer        string `json:"signer"`
	Taker         string `json:"taker"`
	TokenID       string `json:"tokenId"`
	MakerAmount   string `json:"makerAmount"`
	TakerAmount   string `json:"takerAmount"`
	Side          string `json:"side"`
	Expiration    string `json:"expiration"`
	Nonce         string `json:"nonce"`
	FeeRateBps    string `json:"feeRateBps"`
	SignatureType int    `json:"signatureType"`
	Signature     string `json:"signature"`
}

// OrderPayload is the JSON body sent to POST /order.
type OrderPayload struct {
	Order     SignedOrder `json:"order"`
	OrderType string     `json:"orderType"` // "GTC", "FOK", etc.
	Owner     string     `json:"owner"`
}

// OrderResponse is returned by the CLOB after submitting an order.
type OrderResponse struct {
	Success  bool   `json:"success"`
	ErrorMsg string `json:"errorMsg"`
	OrderID  string `json:"orderID"`
	Status   string `json:"status"`
}

// OrderStatus represents the current state of an order on the CLOB.
type OrderStatus struct {
	ID     string `json:"id"`
	Status string `json:"status"` // "live", "matched", "cancelled"
}

// Client is a Polymarket CLOB REST client capable of building, signing, and
// submitting orders, as well as polling for their fill status.
type Client struct {
	httpClient *http.Client
	creds      *APICredentials
	address    common.Address
	privateKey *ecdsa.PrivateKey
}

// NewClient constructs a Client with the given credentials and signing key.
func NewClient(creds *APICredentials, privateKey *ecdsa.PrivateKey, address common.Address) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		creds:      creds,
		address:    address,
		privateKey: privateKey,
	}
}

// BuildAndSignOrder constructs the EIP-712 typed data for a CTF Exchange order,
// signs it with the client's private key, and returns the populated SignedOrder.
//
// Amount conventions (all values scaled to 6 decimals, i.e. USDC / share units):
//   - BUY:  makerAmount = price * size * 1e6 (USDC in),  takerAmount = size * 1e6 (shares out)
//   - SELL: makerAmount = size * 1e6 (shares in),        takerAmount = price * size * 1e6 (USDC out)
func (c *Client) BuildAndSignOrder(req OrderRequest) (*SignedOrder, error) {
	// --- Salt: random uint256 ---
	saltBytes := make([]byte, 32)
	if _, err := rand.Read(saltBytes); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	salt := new(big.Int).SetBytes(saltBytes)

	// --- Amounts ---
	// Use big.Float throughout to avoid float64 precision loss.
	scale := new(big.Float).SetFloat64(1e6)
	price := new(big.Float).SetFloat64(req.Price)
	size := new(big.Float).SetFloat64(req.Size)

	var makerAmount, takerAmount *big.Int
	// usdcRaw = price * size * 1e6
	usdcAmount := new(big.Float).Mul(new(big.Float).Mul(price, size), scale)
	// shareRaw = size * 1e6
	shareAmount := new(big.Float).Mul(size, scale)

	usdcInt, _ := usdcAmount.Int(nil)
	shareInt, _ := shareAmount.Int(nil)

	switch req.Side {
	case Buy:
		makerAmount = usdcInt
		takerAmount = shareInt
	case Sell:
		makerAmount = shareInt
		takerAmount = usdcInt
	default:
		return nil, fmt.Errorf("unknown order side: %d", req.Side)
	}

	tokenID, ok := new(big.Int).SetString(req.TokenID, 10)
	if !ok {
		return nil, fmt.Errorf("invalid tokenID (must be decimal integer): %q", req.TokenID)
	}

	zeroAddr := common.Address{}
	expiration := big.NewInt(0)
	nonce := big.NewInt(0)
	feeRateBps := big.NewInt(0)
	sideVal := big.NewInt(int64(req.Side))
	signatureType := big.NewInt(0) // 0 = EOA

	// --- EIP-712 domain separator ---
	// keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)")
	domainTypeHash := crypto.Keccak256Hash([]byte(orderDomainTypeString))

	// keccak256("CTF Exchange")
	nameHash := crypto.Keccak256Hash([]byte(orderDomainName))

	// keccak256("1")
	versionHash := crypto.Keccak256Hash([]byte(orderDomainVersion))

	chainIDBig := big.NewInt(PolygonChainID)
	contractAddr := common.HexToAddress(CTFExchange)

	// abi.encode(domainTypeHash, nameHash, versionHash, chainId, verifyingContract)
	domainEncoded := make([]byte, 0, 160)
	domainEncoded = append(domainEncoded, domainTypeHash.Bytes()...)
	domainEncoded = append(domainEncoded, nameHash.Bytes()...)
	domainEncoded = append(domainEncoded, versionHash.Bytes()...)
	domainEncoded = append(domainEncoded, common.LeftPadBytes(chainIDBig.Bytes(), 32)...)
	// address is left-padded to 32 bytes (12 zero bytes + 20-byte address)
	domainEncoded = append(domainEncoded, common.LeftPadBytes(contractAddr.Bytes(), 32)...)

	domainSeparator := crypto.Keccak256Hash(domainEncoded)

	// --- Order struct hash ---
	// keccak256("Order(uint256 salt,address maker,address signer,address taker,uint256 tokenId,uint256 makerAmount,uint256 takerAmount,uint256 expiration,uint256 nonce,uint256 feeRateBps,uint8 side,uint8 signatureType)")
	orderTypeHash := crypto.Keccak256Hash([]byte(orderTypeString))

	// abi.encode all order fields in declaration order, each padded to 32 bytes.
	structEncoded := make([]byte, 0, 384)
	structEncoded = append(structEncoded, orderTypeHash.Bytes()...)
	structEncoded = append(structEncoded, common.LeftPadBytes(salt.Bytes(), 32)...)
	structEncoded = append(structEncoded, common.LeftPadBytes(c.address.Bytes(), 32)...)  // maker
	structEncoded = append(structEncoded, common.LeftPadBytes(c.address.Bytes(), 32)...)  // signer
	structEncoded = append(structEncoded, common.LeftPadBytes(zeroAddr.Bytes(), 32)...)   // taker
	structEncoded = append(structEncoded, common.LeftPadBytes(tokenID.Bytes(), 32)...)
	structEncoded = append(structEncoded, common.LeftPadBytes(makerAmount.Bytes(), 32)...)
	structEncoded = append(structEncoded, common.LeftPadBytes(takerAmount.Bytes(), 32)...)
	structEncoded = append(structEncoded, common.LeftPadBytes(expiration.Bytes(), 32)...)
	structEncoded = append(structEncoded, common.LeftPadBytes(nonce.Bytes(), 32)...)
	structEncoded = append(structEncoded, common.LeftPadBytes(feeRateBps.Bytes(), 32)...)
	structEncoded = append(structEncoded, common.LeftPadBytes(sideVal.Bytes(), 32)...)
	structEncoded = append(structEncoded, common.LeftPadBytes(signatureType.Bytes(), 32)...)

	structHash := crypto.Keccak256Hash(structEncoded)

	// --- Final EIP-712 digest ---
	// keccak256("\x19\x01" || domainSeparator || structHash)
	finalData := make([]byte, 0, 66)
	finalData = append(finalData, []byte("\x19\x01")...)
	finalData = append(finalData, domainSeparator.Bytes()...)
	finalData = append(finalData, structHash.Bytes()...)

	digest := crypto.Keccak256Hash(finalData)

	// Sign — crypto.Sign returns [R || S || V] where V is 0 or 1.
	sig, err := crypto.Sign(digest.Bytes(), c.privateKey)
	if err != nil {
		return nil, fmt.Errorf("sign EIP-712 digest: %w", err)
	}
	// Adjust V to Ethereum convention (add 27) for external consumption.
	sig[64] += 27
	sigHex := "0x" + common.Bytes2Hex(sig)

	sideStr := "BUY"
	if req.Side == Sell {
		sideStr = "SELL"
	}

	return &SignedOrder{
		Salt:          salt.String(),
		Maker:         c.address.Hex(),
		Signer:        c.address.Hex(),
		Taker:         zeroAddr.Hex(),
		TokenID:       req.TokenID,
		MakerAmount:   makerAmount.String(),
		TakerAmount:   takerAmount.String(),
		Side:          sideStr,
		Expiration:    expiration.String(),
		Nonce:         nonce.String(),
		FeeRateBps:    feeRateBps.String(),
		SignatureType: 0,
		Signature:     sigHex,
	}, nil
}

// SubmitOrder POSTs a signed order to the Polymarket CLOB API using L2 authentication.
func (c *Client) SubmitOrder(ctx context.Context, signed *SignedOrder, orderType string) (*OrderResponse, error) {
	payload := OrderPayload{
		Order:     *signed,
		OrderType: orderType,
		Owner:     c.address.Hex(),
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal order payload: %w", err)
	}

	const path = "/order"
	l2Headers := c.creds.SignL2Request("POST", path, string(bodyBytes), c.address)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, clobBaseURL+path, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create POST /order request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, vals := range l2Headers {
		for _, v := range vals {
			req.Header.Set(key, v)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST /order: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read /order response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("POST /order returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var orderResp OrderResponse
	if err := json.Unmarshal(respBody, &orderResp); err != nil {
		return nil, fmt.Errorf("parse /order response: %w", err)
	}

	if !orderResp.Success {
		return &orderResp, fmt.Errorf("order submission failed: %s", orderResp.ErrorMsg)
	}

	return &orderResp, nil
}

// GetOrderStatus fetches the current status of an order by its ID.
func (c *Client) GetOrderStatus(ctx context.Context, orderID string) (*OrderStatus, error) {
	path := "/data/order/" + orderID
	l2Headers := c.creds.SignL2Request("GET", path, "", c.address)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clobBaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create GET %s request: %w", path, err)
	}
	for key, vals := range l2Headers {
		for _, v := range vals {
			req.Header.Set(key, v)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read order status response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned status %d: %s", path, resp.StatusCode, string(respBody))
	}

	var status OrderStatus
	if err := json.Unmarshal(respBody, &status); err != nil {
		return nil, fmt.Errorf("parse order status response: %w", err)
	}

	return &status, nil
}

// PollOrderFill polls GetOrderStatus at the given interval until the order is
// matched/filled or cancelled, or until the timeout elapses.
// It returns the final OrderStatus when a terminal state is reached.
func (c *Client) PollOrderFill(ctx context.Context, orderID string, pollInterval, timeout time.Duration) (*OrderStatus, error) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-deadline:
			return nil, fmt.Errorf("timed out after %s waiting for order %s to fill", timeout, orderID)

		case <-ticker.C:
			status, err := c.GetOrderStatus(ctx, orderID)
			if err != nil {
				// Transient errors are retried; the caller's context or deadline
				// will ultimately bound the retry loop.
				fmt.Printf("  [polymarket] poll error for order %s (will retry): %v\n", orderID, err)
				continue
			}

			fmt.Printf("  [polymarket] order %s status: %s\n", orderID, status.Status)

			switch status.Status {
			case "matched", "filled":
				return status, nil
			case "cancelled", "canceled":
				return status, fmt.Errorf("order %s was cancelled", orderID)
			}
			// "live" and any other interim states: keep polling.
		}
	}
}
