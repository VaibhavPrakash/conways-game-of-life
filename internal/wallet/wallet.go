package wallet

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	MonadChainID   = 143
	PolygonChainID = 137
	MonadRPC       = "https://rpc.monad.xyz"
	PolygonRPC     = "https://polygon-rpc.com"

	USDCMonad   = "0x754704Bc059F8C67012fEd69BC8A327a5aafb603"
	USDCePolygon = "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174"
)

// ERC20 minimal ABI for balanceOf and approve.
var erc20ABI abi.ABI

func init() {
	parsed, err := abi.JSON(strings.NewReader(`[
		{"constant":true,"inputs":[{"name":"account","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"},
		{"constant":false,"inputs":[{"name":"spender","type":"address"},{"name":"amount","type":"uint256"}],"name":"approve","outputs":[{"name":"","type":"bool"}],"type":"function"},
		{"constant":true,"inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"name":"allowance","outputs":[{"name":"","type":"uint256"}],"type":"function"}
	]`))
	if err != nil {
		panic(err)
	}
	erc20ABI = parsed
}

type Wallet struct {
	PrivateKey *ecdsa.PrivateKey
	Address    common.Address
}

func FromPrivateKey(hexKey string) (*Wallet, error) {
	hexKey = strings.TrimPrefix(hexKey, "0x")
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)
	return &Wallet{PrivateKey: key, Address: addr}, nil
}

func (w *Wallet) BalanceOf(ctx context.Context, rpcURL string, tokenAddr string) (*big.Int, error) {
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", rpcURL, err)
	}
	defer client.Close()

	data, err := erc20ABI.Pack("balanceOf", w.Address)
	if err != nil {
		return nil, err
	}

	token := common.HexToAddress(tokenAddr)
	result, err := client.CallContract(ctx, ethereum.CallMsg{
		To:   &token,
		Data: data,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("balanceOf call: %w", err)
	}

	outputs, err := erc20ABI.Unpack("balanceOf", result)
	if err != nil {
		return nil, err
	}
	return outputs[0].(*big.Int), nil
}

func (w *Wallet) Allowance(ctx context.Context, rpcURL string, tokenAddr, spenderAddr string) (*big.Int, error) {
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", rpcURL, err)
	}
	defer client.Close()

	data, err := erc20ABI.Pack("allowance", w.Address, common.HexToAddress(spenderAddr))
	if err != nil {
		return nil, err
	}

	token := common.HexToAddress(tokenAddr)
	result, err := client.CallContract(ctx, ethereum.CallMsg{
		To:   &token,
		Data: data,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("allowance call: %w", err)
	}

	outputs, err := erc20ABI.Unpack("allowance", result)
	if err != nil {
		return nil, err
	}
	return outputs[0].(*big.Int), nil
}

func (w *Wallet) Approve(ctx context.Context, rpcURL string, chainID int64, tokenAddr, spenderAddr string, amount *big.Int) (common.Hash, error) {
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return common.Hash{}, fmt.Errorf("dial %s: %w", rpcURL, err)
	}
	defer client.Close()

	data, err := erc20ABI.Pack("approve", common.HexToAddress(spenderAddr), amount)
	if err != nil {
		return common.Hash{}, err
	}

	return w.sendTx(ctx, client, chainID, common.HexToAddress(tokenAddr), data, nil)
}

func (w *Wallet) SendTx(ctx context.Context, rpcURL string, chainID int64, to common.Address, data []byte, value *big.Int) (common.Hash, error) {
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return common.Hash{}, fmt.Errorf("dial %s: %w", rpcURL, err)
	}
	defer client.Close()

	return w.sendTx(ctx, client, chainID, to, data, value)
}

func (w *Wallet) sendTx(ctx context.Context, client *ethclient.Client, chainID int64, to common.Address, data []byte, value *big.Int) (common.Hash, error) {
	nonce, err := client.PendingNonceAt(ctx, w.Address)
	if err != nil {
		return common.Hash{}, fmt.Errorf("get nonce: %w", err)
	}

	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("gas price: %w", err)
	}

	if value == nil {
		value = big.NewInt(0)
	}

	gasLimit, err := client.EstimateGas(ctx, ethereum.CallMsg{
		From:  w.Address,
		To:    &to,
		Data:  data,
		Value: value,
	})
	if err != nil {
		return common.Hash{}, fmt.Errorf("estimate gas: %w", err)
	}

	tx := types.NewTransaction(nonce, to, value, gasLimit, gasPrice, data)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(big.NewInt(chainID)), w.PrivateKey)
	if err != nil {
		return common.Hash{}, fmt.Errorf("sign tx: %w", err)
	}

	if err := client.SendTransaction(ctx, signedTx); err != nil {
		return common.Hash{}, fmt.Errorf("send tx: %w", err)
	}

	return signedTx.Hash(), nil
}

func (w *Wallet) WaitForTx(ctx context.Context, rpcURL string, txHash common.Hash) (*types.Receipt, error) {
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", rpcURL, err)
	}
	defer client.Close()

	// Use the built-in TransactionReceipt polling.
	for {
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err == nil {
			return receipt, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
}

// FormatUSDC formats a raw USDC amount (6 decimals) as a human-readable string.
func FormatUSDC(amount *big.Int) string {
	if amount == nil {
		return "0.000000"
	}
	d := new(big.Float).SetInt(amount)
	d.Quo(d, big.NewFloat(1e6))
	return fmt.Sprintf("%.6f", d)
}

// ParseUSDC converts a human-readable USDC amount to raw (6 decimals).
func ParseUSDC(amount float64) *big.Int {
	raw := new(big.Float).SetFloat64(amount)
	raw.Mul(raw, big.NewFloat(1e6))
	i, _ := raw.Int(nil)
	return i
}
