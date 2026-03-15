package polymarket

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	clobBaseURL = "https://clob.polymarket.com"

	// EIP-712 type strings for hashing.
	eip712DomainTypeString = "EIP712Domain(string name,string version,uint256 chainId)"
	clobAuthTypeString     = "ClobAuth(address address,string timestamp,uint256 nonce,string message)"

	domainName    = "ClobAuthDomain"
	domainVersion = "1"
	chainID       = uint64(137)

	l1Message = "This message attests that I control the given wallet"
)

// APICredentials holds the API key material returned by Polymarket after L1 auth.
type APICredentials struct {
	APIKey     string
	Secret     string
	Passphrase string
}

// deriveAPIKeyResponse is the JSON shape returned by POST /auth/derive-api-key.
type deriveAPIKeyResponse struct {
	APIKey     string `json:"apiKey"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

// buildL1Headers constructs the EIP-712 signed headers required for L1 authentication.
func buildL1Headers(privateKey *ecdsa.PrivateKey, address common.Address) (http.Header, error) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := big.NewInt(0)

	// --- Domain separator ---
	// keccak256("EIP712Domain(string name,string version,uint256 chainId)")
	domainTypeHash := crypto.Keccak256Hash([]byte(eip712DomainTypeString))

	// keccak256("ClobAuthDomain")
	nameHash := crypto.Keccak256Hash([]byte(domainName))

	// keccak256("1")
	versionHash := crypto.Keccak256Hash([]byte(domainVersion))

	// abi.encode(domainTypeHash, nameHash, versionHash, chainId)
	// Each element is padded to 32 bytes.
	chainIDBig := new(big.Int).SetUint64(chainID)
	domainEncoded := make([]byte, 0, 128)
	domainEncoded = append(domainEncoded, domainTypeHash.Bytes()...)
	domainEncoded = append(domainEncoded, nameHash.Bytes()...)
	domainEncoded = append(domainEncoded, versionHash.Bytes()...)
	domainEncoded = append(domainEncoded, common.LeftPadBytes(chainIDBig.Bytes(), 32)...)

	domainSeparator := crypto.Keccak256Hash(domainEncoded)

	// --- Struct hash ---
	// keccak256("ClobAuth(address address,string timestamp,uint256 nonce,string message)")
	structTypeHash := crypto.Keccak256Hash([]byte(clobAuthTypeString))

	// keccak256(timestamp)
	timestampHash := crypto.Keccak256Hash([]byte(timestamp))

	// keccak256(message)
	messageHash := crypto.Keccak256Hash([]byte(l1Message))

	// abi.encode(structTypeHash, address (padded to 32), keccak256(timestamp), nonce (padded to 32), keccak256(message))
	structEncoded := make([]byte, 0, 160)
	structEncoded = append(structEncoded, structTypeHash.Bytes()...)
	// address is encoded as a 32-byte left-padded value (12 zero bytes + 20 address bytes)
	structEncoded = append(structEncoded, common.LeftPadBytes(address.Bytes(), 32)...)
	structEncoded = append(structEncoded, timestampHash.Bytes()...)
	structEncoded = append(structEncoded, common.LeftPadBytes(nonce.Bytes(), 32)...)
	structEncoded = append(structEncoded, messageHash.Bytes()...)

	structHash := crypto.Keccak256Hash(structEncoded)

	// --- Final EIP-712 hash ---
	// keccak256("\x19\x01" + domainSeparator + structHash)
	finalData := make([]byte, 0, 66)
	finalData = append(finalData, []byte("\x19\x01")...)
	finalData = append(finalData, domainSeparator.Bytes()...)
	finalData = append(finalData, structHash.Bytes()...)

	digest := crypto.Keccak256Hash(finalData)

	// Sign the digest. crypto.Sign returns a 65-byte [R || S || V] signature.
	sig, err := crypto.Sign(digest.Bytes(), privateKey)
	if err != nil {
		return nil, fmt.Errorf("sign EIP-712 digest: %w", err)
	}

	// Adjust V to Ethereum convention (add 27) — crypto.Sign returns V as 0/1.
	sig[64] += 27

	// Polymarket expects the signature as a 0x-prefixed hex string.
	sigHex := "0x" + common.Bytes2Hex(sig)

	headers := http.Header{}
	headers.Set("POLY_ADDRESS", address.Hex())
	headers.Set("POLY_SIGNATURE", sigHex)
	headers.Set("POLY_TIMESTAMP", timestamp)
	headers.Set("POLY_NONCE", nonce.String())

	return headers, nil
}

// DeriveAPICredentials performs L1 authentication and fetches API credentials from Polymarket.
func DeriveAPICredentials(ctx context.Context, privateKey *ecdsa.PrivateKey, address common.Address) (*APICredentials, error) {
	l1Headers, err := buildL1Headers(privateKey, address)
	if err != nil {
		return nil, fmt.Errorf("build L1 headers: %w", err)
	}

	reqBody := bytes.NewBufferString("{}")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, clobBaseURL+"/auth/derive-api-key", reqBody)
	if err != nil {
		return nil, fmt.Errorf("create derive-api-key request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, vals := range l1Headers {
		for _, v := range vals {
			req.Header.Set(key, v)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST /auth/derive-api-key: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read derive-api-key response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("derive-api-key returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	var parsed deriveAPIKeyResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("parse derive-api-key response: %w", err)
	}

	if parsed.APIKey == "" || parsed.Secret == "" || parsed.Passphrase == "" {
		return nil, fmt.Errorf("derive-api-key response missing fields: %s", string(respBytes))
	}

	return &APICredentials{
		APIKey:     parsed.APIKey,
		Secret:     parsed.Secret,
		Passphrase: parsed.Passphrase,
	}, nil
}

// SignL2Request builds the HMAC-signed HTTP headers required for L2 authenticated
// requests. address is the wallet address to include in the POLY_ADDRESS header.
func (c *APICredentials) SignL2Request(method, path, body string, address common.Address) http.Header {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// HMAC-SHA256 of timestamp + method + path + body, keyed by base64-decoded secret.
	secretBytes, err := base64.StdEncoding.DecodeString(c.Secret)
	if err != nil {
		// Try URL-safe base64 before falling back to raw bytes.
		secretBytes, err = base64.URLEncoding.DecodeString(c.Secret)
		if err != nil {
			fmt.Printf("  [warning] API secret is not valid base64, using raw bytes\n")
			secretBytes = []byte(c.Secret)
		}
	}

	message := timestamp + method + path + body

	mac := hmac.New(sha256.New, secretBytes)
	mac.Write([]byte(message))
	sigBytes := mac.Sum(nil)
	sig := base64.StdEncoding.EncodeToString(sigBytes)

	headers := http.Header{}
	headers.Set("POLY_ADDRESS", address.Hex())
	headers.Set("POLY_SIGNATURE", sig)
	headers.Set("POLY_TIMESTAMP", timestamp)
	headers.Set("POLY_NONCE", "0")
	headers.Set("POLY_API_KEY", c.APIKey)
	headers.Set("POLY_PASSPHRASE", c.Passphrase)

	return headers
}
