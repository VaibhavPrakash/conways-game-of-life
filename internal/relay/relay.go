package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

const (
	BaseURL = "https://api.relay.link"
)

type Client struct {
	httpClient *http.Client
	apiKey     string
}

func NewClient(apiKey string) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiKey:     apiKey,
	}
}

// QuoteRequest is the payload for POST /quote/v2.
type QuoteRequest struct {
	User                string `json:"user"`
	OriginChainID       int    `json:"originChainId"`
	DestinationChainID  int    `json:"destinationChainId"`
	OriginCurrency      string `json:"originCurrency"`
	DestinationCurrency string `json:"destinationCurrency"`
	Amount              string `json:"amount"`
	TradeType           string `json:"tradeType"`
}

// QuoteResponse contains the steps to execute the bridge.
type QuoteResponse struct {
	Steps []Step `json:"steps"`
	// Additional fields we care about for timing.
	Details json.RawMessage `json:"details"`
}

type Step struct {
	ID          string       `json:"id"`
	Action      string       `json:"action"`
	Description string       `json:"description"`
	Kind        string       `json:"kind"`
	RequestID   string       `json:"requestId"`
	Items       []StepItem   `json:"items"`
}

type StepItem struct {
	Status string  `json:"status"`
	Data   TxData  `json:"data"`
}

type TxData struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Data     string `json:"data"`
	Value    string `json:"value"`
	ChainID  int    `json:"chainId"`
	MaxFeePerGas         string `json:"maxFeePerGas,omitempty"`
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas,omitempty"`
	Gas      string `json:"gas,omitempty"`
}

// IntentStatus is the response from GET /intents/status/v3.
type IntentStatus struct {
	Status string `json:"status"`
	// "pending", "success", "failure"
	Details json.RawMessage `json:"details,omitempty"`
}

func (c *Client) GetQuote(ctx context.Context, req QuoteRequest) (*QuoteResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", BaseURL+"/quote/v2", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("relay quote request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("relay quote failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var quoteResp QuoteResponse
	if err := json.Unmarshal(respBody, &quoteResp); err != nil {
		return nil, fmt.Errorf("parse quote response: %w", err)
	}
	return &quoteResp, nil
}

// GetStepTxData extracts the transaction data from a quote step.
// Returns the TxData for signing and submitting.
func GetStepTxData(step Step) (*TxData, string, error) {
	if len(step.Items) == 0 {
		return nil, "", fmt.Errorf("step %q has no items", step.ID)
	}
	item := step.Items[0]
	if item.Status == "failed" || item.Status == "error" {
		return nil, "", fmt.Errorf("step %q first item has status %q", step.ID, item.Status)
	}
	return &item.Data, step.RequestID, nil
}

// ParseTxDataTo returns the "to" address from TxData.
func ParseTxDataTo(td *TxData) common.Address {
	return common.HexToAddress(td.To)
}

func (c *Client) PollStatus(ctx context.Context, requestID string, pollInterval time.Duration, timeout time.Duration) (*IntentStatus, error) {
	deadline := time.After(timeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, fmt.Errorf("bridge timeout after %s", timeout)
		case <-ticker.C:
			status, err := c.getStatus(ctx, requestID)
			if err != nil {
				fmt.Printf("  [relay] poll error (will retry): %v\n", err)
				continue
			}
			fmt.Printf("  [relay] bridge status: %s\n", status.Status)
			switch status.Status {
			case "success":
				return status, nil
			case "failure", "failed":
				return status, fmt.Errorf("bridge failed")
			}
		}
	}
}

func (c *Client) getStatus(ctx context.Context, requestID string) (*IntentStatus, error) {
	statusURL := fmt.Sprintf("%s/intents/status/v3?requestId=%s", BaseURL, url.QueryEscape(requestID))
	req, err := http.NewRequestWithContext(ctx, "GET", statusURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status check failed (%d): %s", resp.StatusCode, string(body))
	}

	var status IntentStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
