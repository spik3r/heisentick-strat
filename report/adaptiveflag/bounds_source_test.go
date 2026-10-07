package adaptiveflag

import (
	"crypto/sha256"
	"fmt"
	"github.com/spik3r/heisentick-strat/testsupport"
	"os"
	"path/filepath"
	"testing"
)

// Any constructor/helper change invalidates the reviewed resource/domain proof.
// Renew a fingerprint only after independent review of the changed proof, not
// merely because a later implementation still passes typical invented cases.
func TestRuntimeReviewedConstructorFingerprints(t *testing.T) {
	root := testsupport.MustRepoRoot()
	expected := map[string]string{
		"engine/adaptive_flag_signals.go":   "53bcd15bc99478fe0b2c4320074055a7bdb7d825b34a4335722e042c76f7cc97",
		"engine/adaptive_flag_reference.go": "4ef043a85c5eaea69d82bf726513a630e417ab788f419518df83111890bc23d7",
		"engine/adaptive_flag_types.go":     "21732bd4779fe8cd6c0493441959e33e3112699f4058b99a64db4dee8465beed",
		"engine/adaptive_flag_research.go":  "2b05e98bd6643ccfb374527d465b21dd697de0f95527589ab4edc11146cb0263",
		"engine/broker.go":                  "2d9d9136906e78bb13c06aec9991e10647c73256b5ccca8772558f923053c272",
		"engine/broker_special_setups.go":   "4240e6a82b7b96fe53a48480ac2affc1821a7acdd164810463f8df6a7d987d13",
		"dsl/adaptive_flag_config.go":       "45bb5cefb226a6919c28ea260eb604be188a1e38717fa9326810d52b34324039",
		"marketdata/btb1.go":                "33b06da51bc378f140a529b5b169295263e1df4e45b9173a98277d4ea35fdc61",
	}
	for path, want := range expected {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != want {
			t.Errorf("%s changed; independent adaptive runtime bound/domain proof review required before renewing %s", path, got)
		}
	}
}
