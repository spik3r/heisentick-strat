// Package seqcore implements the owner-accepted synthetic Sequential research
// convention seq-core-freeze.v1. It is not proprietary DeMARK parity, a trading
// policy, a broker, a real-market calendar, or a production DSL integration.
//
// Construct with DefaultConfig and a pinned Identity, then feed closed bars in
// increasing open-time order. Step returns detached snapshots and ordered events.
// An Engine is single-stream and is not safe for concurrent mutation. Independent
// engines may run concurrently. No market data or economic outcomes are used.
//
// Unexpected gaps clear Setup state and unfinished Countdowns, preserve completed
// Countdowns, and restart comparison warmup. This is the explicitly approved
// synthetic lifecycle amendment, not an inherited DeMARK rule. TDST range export,
// TDST cancellation, Risk Level, Combo, Intersection and trading execution remain
// excluded. ResponseWindowPolicy never mutates lifecycle state and knows no fills.
//
// The authoritative fixtures and their source digests are vendored in testdata.
// The canonical task is https://github.com/spik3r/heisentick-backlog/pull/457;
// acceptance is recorded in backlog commit bf7a88c8d79919190254ca6fa04a865fbe914657.
package seqcore
