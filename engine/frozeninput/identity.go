// Package frozeninput binds bounded original input bytes to compiled frozen
// controls. It preserves declarations without qualifying a source for execution.
package frozeninput

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/spik3r/heisentick-strat/dsl"
)

const MaxInputBytes = 1 << 20

// Documents contains original JSON bytes, including whitespace and final LF.
type Documents struct{ Snapshot, Source, Ordering []byte }

// Artifact is one opaque local leaf declared by the source. No fetching or
// format decoding is performed. Ref aliases are declarations, not intrinsic IDs.
type Artifact struct {
	Ref   dsl.FrozenQuoteRef
	Bytes []byte
}

func digest(b []byte) string    { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func copyBytes(b []byte) []byte { return append([]byte(nil), b...) }
func copyDocuments(d Documents) Documents {
	return Documents{copyBytes(d.Snapshot), copyBytes(d.Source), copyBytes(d.Ordering)}
}
func artifactKey(r dsl.FrozenQuoteRef) string { return r.ID + "/" + r.Version }
func documentRef(id, version string, raw []byte) dsl.FrozenQuoteRef {
	return dsl.FrozenQuoteRef{ID: id, Version: version, SHA256: digest(raw)}
}

// checkBudget subtracts actual byte lengths, so even oversized caller slices
// cannot overflow an aggregate sum. It never uses declared record/byte counts.
func checkBudget(config []byte, d Documents, artifacts []Artifact) error {
	remaining := MaxInputBytes
	consume := func(n int) bool {
		if n > remaining {
			return false
		}
		remaining -= n
		return true
	}
	for _, b := range [][]byte{config, d.Snapshot, d.Source, d.Ordering} {
		if !consume(len(b)) {
			return fmt.Errorf("input-size: aggregate input exceeds 1 MiB")
		}
	}
	for _, a := range artifacts {
		if !consume(len(a.Bytes)) {
			return fmt.Errorf("input-size: aggregate input exceeds 1 MiB")
		}
	}
	return nil
}

// The config has already been independently normalized; only this private
// helper reads its known typed reference projection, never the caller map.
func configRef(cfg dsl.Config, name string) dsl.FrozenQuoteRef {
	m := cfg["frozenLevelBreakout"].(map[string]any)["inputRefs"].(map[string]any)[name].(map[string]any)
	return dsl.FrozenQuoteRef{ID: m["id"].(string), Version: m["version"].(string), SHA256: m["sha256"].(string)}
}
