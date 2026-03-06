package gnoduty

// chain-details.go
// GnoDuty: The cosmos directory registry is not used for Gno.land chains.
// This file is kept minimal for compatibility with any remaining references.

// altValopers is kept empty — Gno.land uses bech32 g1... addresses directly.
type valoperOverrides struct {
	Prefixes map[string]string `json:"prefixes"`
}

var altValopers = &valoperOverrides{
	Prefixes: map[string]string{},
}
