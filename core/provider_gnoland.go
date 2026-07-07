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
				Data  string          `json:"Data"`
				Log   string          `json:"Log"`
				Error json.RawMessage `json:"Error"`
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

// valoperAddrRex matches .uverse.address fields in the struct returned by GetByAddr
var valoperAddrRex = regexp.MustCompile(`"(g1[a-z0-9]+)" \.uverse\.address`)

// GnoResolveConsensusAddr resolves a valoper account address to its consensus address
// by querying the valopers realm. The consensus address is the second .uverse.address
// field in the GetByAddr response (first is the account address itself).
func GnoResolveConsensusAddr(rpcURL, realmPath, accountAddr string) (string, error) {
	expr := fmt.Sprintf(`%s.GetByAddr("%s")`, realmPath, accountAddr)
	dataHex := hex.EncodeToString([]byte(expr))

	qurl := fmt.Sprintf(`%s/abci_query?path=%%22vm/qeval%%22&data=0x%s`,
		strings.TrimRight(rpcURL, "/"), dataHex)

	body, err := gnoHTTPGet(qurl, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("qeval request failed: %w", err)
	}

	var result GnoABCIResult
	if err = json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("qeval unmarshal failed: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("qeval error: %s", result.Error.Message)
	}

	if result.Result == nil {
		return "", errors.New("qeval: nil result")
	}

	dataB64 := result.Result.Response.ResponseBase.Data
	if dataB64 == "" {
		return "", errors.New("qeval: empty response data")
	}

	decoded, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		return "", fmt.Errorf("qeval base64 decode failed: %w", err)
	}

	// Extract all .uverse.address fields
	// First match = account address, second match = consensus address
	matches := valoperAddrRex.FindAllStringSubmatch(string(decoded), -1)
	if len(matches) >= 2 && len(matches[1]) >= 2 {
		return matches[1][1], nil
	}

	return "", errors.New("consensus address not found in valopers response")
}

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

	realmPath := cc.GnoValopersRealm
	if realmPath == "" {
		realmPath = "gno.land/r/gnops/valopers"
	}

	// 1. Resolve consensus address from valopers realm (once)
	if cc.gnoConsensusAddr == "" {
		// First try direct match (valoper_address might already be a consensus address)
		directMatch, _ := GnoIsValidatorActive(rpcURL, cc.ValAddress)
		if directMatch {
			cc.gnoConsensusAddr = cc.ValAddress
		} else {
			// Resolve account address → consensus address via valopers realm
			consAddr, err := GnoResolveConsensusAddr(rpcURL, realmPath, cc.ValAddress)
			if err != nil {
				if first {
					l(fmt.Sprintf("⚠️ could not resolve consensus address for %s: %s", cc.ValAddress, err))
				}
			} else {
				cc.gnoConsensusAddr = consAddr
				if first {
					l(fmt.Sprintf("⚙️ resolved %s → consensus %s", cc.ValAddress, consAddr))
				}
			}
		}
	}

	// 2. Check if validator is in the active set using consensus address
	lookupAddr := cc.gnoConsensusAddr
	if lookupAddr == "" {
		lookupAddr = cc.ValAddress // fallback
	}
	bonded, err := GnoIsValidatorActive(rpcURL, lookupAddr)
	if err != nil {
		if first {
			l(fmt.Sprintf("⚠️ could not check active set for %s: %s", lookupAddr, err))
		}
		// Keep previous bonded state on RPC error
	} else {
		cc.valInfo.Bonded = bonded
	}

	// 3. Resolve moniker via valopers realm (using account address, not consensus)
	moniker, err := GnoGetMoniker(rpcURL, realmPath, cc.ValAddress)
	if err != nil {
		if first {
			l(fmt.Sprintf("⚠️ could not resolve moniker for %s: %s", cc.ValAddress, err))
		}
		moniker = cc.ValAddress[:20] + "..."
	}
	cc.valInfo.Moniker = moniker

	// 4. Gno.land has no slashing module
	cc.valInfo.Jailed = false
	cc.valInfo.Tombstoned = false

	// 5. Store consensus address for block signature matching
	if len(cc.valInfo.Conspub) == 0 && cc.gnoConsensusAddr != "" {
		vals, verr := GnoGetValidators(rpcURL)
		if verr == nil {
			addrLower := strings.ToLower(cc.gnoConsensusAddr)
			for _, v := range vals {
				if strings.ToLower(v.Address) == addrLower {
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
