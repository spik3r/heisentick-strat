package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// SignalIDLength is the number of hex characters kept from the sha256
// digest ComputeSignalID produces (16 bytes / 128 bits).
const SignalIDLength = 32

// ComputeSignalID derives a stable id for the closed decision bar that
// produced a trade's entry signal:
//
//	sha256(strategyID + "|" + strategyVersion + "|" + symbol + "|" + tf +
//	       "|" + signalBarTs + "|" + side)
//
// truncated to the first SignalIDLength hex characters, per
// heisentick-backlog's plans/2026-09-26-meta-labeling-existing-strategies.md
// §2.3. strategyVersion should be the sha256 of the .strat source bytes (the
// existing module-byte-pinning convention), so the id changes with the
// source even when strategyID is reused across parameter revisions.
//
// signalBarTs uses the trade's entry timestamp (engine.Trade.EntryT, ms
// since the Unix epoch) as the deterministic proxy for the signal decision
// bar: report.Trade carries no separate signal-time field distinct from the
// fill today (see that plan's §6 open questions). Because a strategy's fill
// timing (same-bar close, same-bar open, or next-bar open) is itself
// deterministic given the source and data, this proxy is stable across two
// independent runs of the same source/data and changes whenever any input
// (strategy identity, version, route, side, or the run producing a
// different entry bar) changes.
func ComputeSignalID(strategyID, strategyVersion, symbol, tf string, signalBarTs int64, side string) string {
	material := fmt.Sprintf("%s|%s|%s|%s|%d|%s", strategyID, strategyVersion, symbol, tf, signalBarTs, side)
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])[:SignalIDLength]
}
