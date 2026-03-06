package gnoduty

import (
	"encoding/hex"
	"strings"
)

// ValInfo holds most of the stats/info used for secondary alarms. It is refreshed roughly every minute.
type ValInfo struct {
	Moniker    string `json:"moniker"`
	Bonded     bool   `json:"bonded"`
	Jailed     bool   `json:"jailed"`
	Tombstoned bool   `json:"tombstoned"`
	Missed     int64  `json:"missed"`
	Window     int64  `json:"window"`
	Conspub    []byte `json:"conspub"`
	Valcons    string `json:"valcons"`
}

// GetValInfo delegates to GnoGetValInfo for Gno.land chains.
// The first bool is used to determine if extra information about the validator should be printed.
func (cc *ChainConfig) GetValInfo(first bool) (err error) {
	return cc.GnoGetValInfo(first)
}

func ToBytes(address string) []byte {
	bz, _ := hex.DecodeString(strings.ToLower(address))
	return bz
}
