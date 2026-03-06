package gnoduty

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ──────────────────────────────────────────────
// Gno.land RPC structures (TM2 / Amino JSON)
// ──────────────────────────────────────────────

// GnoStatusResult represents the /status RPC response from a TM2 node.
type GnoStatusResult struct {
	JSONRPC string `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  struct {
		NodeInfo struct {
			Network string `json:"network"`
		} `json:"node_info"`
		SyncInfo struct {
			LatestBlockHeight string `json:"latest_block_height"`
			CatchingUp        bool   `json:"catching_up"`
		} `json:"sync_info"`
	} `json:"result"`
}

// GnoValidatorsResult represents the /validators RPC response.
type GnoValidatorsResult struct {
	JSONRPC string `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  struct {
		Validators []GnoValidator `json:"validators"`
	} `json:"result"`
}

// GnoValidator represents a single validator from the /validators endpoint.
type GnoValidator struct {
	Address  string `json:"address"`
	PubKey   struct {
		Type  string `json:"@type"`
		Value string `json:"value"`
	} `json:"pub_key"`
	VotingPower      string `json:"voting_power"`
	ProposerPriority string `json:"proposer_priority"`
}

// GnoABCIResult represents an abci_query RPC response.
type GnoABCIResult struct {
	JSONRPC string `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  *struct {
		Response struct {
			ResponseBase struct {
				Data  string `json:"Data"`
				Log   string `json:"Log"`
				Error string `json:"Error"`
			} `json:"ResponseBase"`
		} `json:"response"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    string `json:"data"`
	} `json:"error"`
}

// ──────────────────────────────────────────────
// HTTP helpers (generic, works with any RPC URL)
// ──────────────────────────────────────────────

// gnoHTTPGet performs a GET request against an RPC endpoint with a timeout.
func gnoHTTPGet(url string, timeout time.Duration) ([]byte, error) {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// ──────────────────────────────────────────────
// Status check (replaces rpchttp.New + Status)
// ──────────────────────────────────────────────

// GnoGetStatus fetches /status from a TM2 RPC node and returns chain_id, latest height, catching_up.
func GnoGetStatus(rpcURL string) (chainID string, height string, catchingUp bool, err error) {
	url := strings.TrimRight(rpcURL, "/") + "/status?"
	body, err := gnoHTTPGet(url, 10*time.Second)
	if err != nil {
		return "", "", false, fmt.Errorf("status request failed: %w", err)
	}
	var result GnoStatusResult
	if err = json.Unmarshal(body, &result); err != nil {
		return "", "", false, fmt.Errorf("status unmarshal failed: %w", err)
	}
	return result.Result.NodeInfo.Network,
		result.Result.SyncInfo.LatestBlockHeight,
		result.Result.SyncInfo.CatchingUp,
		nil
}

// ──────────────────────────────────────────────
// Validator set lookup
// ──────────────────────────────────────────────

// GnoGetValidators fetches the active validator set from /validators.
// Returns a map of address (uppercase hex or bech32) → GnoValidator.
func GnoGetValidators(rpcURL string) ([]GnoValidator, error) {
	url := strings.TrimRight(rpcURL, "/") + "/validators?per_page=100"
	body, err := gnoHTTPGet(url, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("validators request failed: %w", err)
	}
	var result GnoValidatorsResult
	if err = json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("validators unmarshal failed: %w", err)
	}
	return result.Result.Validators, nil
}

// GnoIsValidatorActive checks if the given address is in the active validator set.
// The address can be bech32 (g1...) or hex — matching is case-insensitive.
func GnoIsValidatorActive(rpcURL string, address string) (bool, error) {
	vals, err := GnoGetValidators(rpcURL)
	if err != nil {
		return false, err
	}
	addrLower := strings.ToLower(address)
	for _, v := range vals {
		if strings.ToLower(v.Address) == addrLower {
			return true, nil
		}
	}
	return false, nil
}

// ──────────────────────────────────────────────
// Moniker resolution via vm/qeval
// ──────────────────────────────────────────────

// valoPerRex matches fields in the struct returned by GetByAddr
var valoperMonikerRex = regexp.MustCompile(`\("([^"]*)" string\)`)

// GnoGetMoniker resolves a validator address to its moniker using the valopers realm.
// realmPath is configurable, e.g. "gno.land/r/gnops/valopers"
func GnoGetMoniker(rpcURL, realmPath, address string) (moniker string, err error) {
	// Build the qeval expression: pkgpath.GetByAddr("address")
	expr := fmt.Sprintf(`%s.GetByAddr("%s")`, realmPath, address)
	dataHex := hex.EncodeToString([]byte(expr))

	url := fmt.Sprintf(`%s/abci_query?path=%%22vm/qeval%%22&data=0x%s`,
		strings.TrimRight(rpcURL, "/"), dataHex)

	body, err := gnoHTTPGet(url, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("qeval request failed: %w", err)
	}

	var result GnoABCIResult
	if err = json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("qeval unmarshal failed: %w", err)
	}

	// Check for JSON-RPC error
	if result.Error != nil {
		return "", fmt.Errorf("qeval error: %s - %s", result.Error.Message, result.Error.Data)
	}

	if result.Result == nil {
		return "", errors.New("qeval: nil result")
	}

	dataB64 := result.Result.Response.ResponseBase.Data
	if dataB64 == "" {
		logMsg := result.Result.Response.ResponseBase.Log
		if logMsg != "" {
			return "", fmt.Errorf("qeval log: %s", logMsg)
		}
		return "", errors.New("qeval: empty response data")
	}

	decoded, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return "", fmt.Errorf("qeval base64 decode failed: %w", err)
	}

	// Parse the struct response: first string field is the moniker
	// Format: (struct{("AviaOne" string),("description" string),...} ...)
	matches := valoperMonikerRex.FindAllStringSubmatch(string(decoded), -1)
	if len(matches) > 0 && len(matches[0]) > 1 {
		return matches[0][1], nil
	}

	return "unknown", nil
}

// ──────────────────────────────────────────────
// GetValInfo for Gno.land chains
// ──────────────────────────────────────────────

// GnoGetValInfo populates the ValInfo for a Gno.land chain.
// It uses HTTP calls to the RPC endpoint (no cosmos-sdk dependency).
func (cc *ChainConfig) GnoGetValInfo(first bool) error {
	if cc.valInfo == nil {
		cc.valInfo = &ValInfo{}
	}

	rpcURL := cc.gnoRPCUrl()
	if rpcURL == "" {
		return errors.New("no RPC URL available")
	}

	// 1. Check if validator is in the active set
	bonded, err := GnoIsValidatorActive(rpcURL, cc.ValAddress)
	if err != nil {
		// Not fatal — we can still monitor blocks
		if first {
			l(fmt.Sprintf("⚠️ could not check active set for %s: %s", cc.ValAddress, err))
		}
	}
	cc.valInfo.Bonded = bonded

	// 2. Resolve moniker via valopers realm
	realmPath := cc.GnoValopersRealm
	if realmPath == "" {
		realmPath = "gno.land/r/gnops/valopers" // default
	}
	moniker, err := GnoGetMoniker(rpcURL, realmPath, cc.ValAddress)
	if err != nil {
		if first {
			l(fmt.Sprintf("⚠️ could not resolve moniker for %s: %s", cc.ValAddress, err))
		}
		moniker = cc.ValAddress[:20] + "..."
	}
	cc.valInfo.Moniker = moniker

	// 3. Gno.land has no slashing module — use local counters
	// Missed and Window are tracked locally via WebSocket block monitoring
	// Jailed is inferred from absence in /validators
	cc.valInfo.Jailed = false // will be detected if validator disappears from set
	cc.valInfo.Tombstoned = false

	// 4. Find the hex address for block signature matching
	// On Gno.land /validators returns bech32 (g1...) addresses
	// But block signatures use hex addresses. We need to map between them.
	if len(cc.valInfo.Conspub) == 0 {
		vals, verr := GnoGetValidators(rpcURL)
		if verr == nil {
			addrLower := strings.ToLower(cc.ValAddress)
			for _, v := range vals {
				if strings.ToLower(v.Address) == addrLower {
					// Store the hex address for block signature matching
					// The address from /validators is what we need
					cc.valInfo.Conspub = []byte(strings.ToUpper(v.Address))
					cc.valInfo.Valcons = v.Address
					break
				}
			}
		}
	}

	if first && cc.valInfo.Bonded {
		l(fmt.Sprintf("⚙️ found %s (%s) in active validator set", cc.ValAddress, cc.valInfo.Moniker))
	} else if first && !cc.valInfo.Bonded {
		l(fmt.Sprintf("❌ %s (%s) is NOT in active validator set", cc.ValAddress, cc.valInfo.Moniker))
	}

	return nil
}

// gnoRPCUrl returns the first available RPC URL for this chain.
func (cc *ChainConfig) gnoRPCUrl() string {
	for _, node := range cc.Nodes {
		if !node.down {
			return node.Url
		}
	}
	// fallback: return the first one even if marked down
	if len(cc.Nodes) > 0 {
		return cc.Nodes[0].Url
	}
	return ""
}
