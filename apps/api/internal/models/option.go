package models

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Option types.
const (
	OptionTypeCall = "call"
	OptionTypePut  = "put"
)

// OptionContractMultiplier is the shares a standard contract covers. Stored per
// holding rather than assumed, because a corporate action can leave a contract
// covering a non-standard number of shares.
const OptionContractMultiplier = 100

// occSymbol matches Fidelity's option symbols, which are OCC-style with a
// leading dash: "-AXP251121C390" is an AXP call, expiring 2025-11-21, struck at
// $390. Strikes can be fractional ("-SLDP250815P2.5").
var occSymbol = regexp.MustCompile(`^-?([A-Z]+)(\d{6})([CP])([0-9]+(?:\.[0-9]+)?)$`)

// OptionDetail is what an option's symbol encodes.
type OptionDetail struct {
	Underlying string
	Expiration time.Time
	Type       string
	Strike     float64
}

// ParseOptionSymbol decodes an OCC-style contract symbol. The second return is
// false for anything that isn't one, so callers can treat a plain ticker
// normally rather than guessing.
func ParseOptionSymbol(symbol string) (OptionDetail, bool) {
	m := occSymbol.FindStringSubmatch(strings.TrimSpace(symbol))
	if m == nil {
		return OptionDetail{}, false
	}

	// Two-digit years in an OCC symbol are this century; these contracts don't
	// predate 2000 and aren't written a century out.
	expiration, err := time.Parse("060102", m[2])
	if err != nil {
		return OptionDetail{}, false
	}

	strike, err := strconv.ParseFloat(m[4], 64)
	if err != nil {
		return OptionDetail{}, false
	}

	optionType := OptionTypeCall
	if m[3] == "P" {
		optionType = OptionTypePut
	}

	return OptionDetail{
		Underlying: m[1],
		Expiration: expiration,
		Type:       optionType,
		Strike:     strike,
	}, true
}
