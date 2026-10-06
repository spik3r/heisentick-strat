package dsl

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestReviewExactIntegerOracle(t *testing.T) {
	raw, e := os.ReadFile("testdata/frozen_level/config.canonical.json")
	if e != nil {
		t.Fatal(e)
	}
	max := new(big.Rat).SetInt64(9007199254740991)
	n := 0
	for _, c := range []string{"0", "1", "2", "9", "10", "11", "99", "100", "1000", "9007199254740990", "9007199254740991", "9007199254740992", "9007199254740993", "90071992547409910", "90071992547409911", "0.01", "0.10", "1.0", "1.01", "9007199254740991.1", "9007199254740990.9"} {
		for _, exp := range []int{-10000, -1000, -324, -308, -17, -16, -15, -3, -2, -1, 0, 1, 2, 3, 15, 16, 17, 308, 324, 1000, 10000} {
			for _, sign := range []string{"", "-"} {
				tok := fmt.Sprintf("%s%se%d", sign, c, exp)
				r, ok := new(big.Rat).SetString(tok)
				if !ok {
					t.Fatal(tok)
				}
				want := r.Sign() >= 0 && r.IsInt() && r.Cmp(max) <= 0 && !(sign == "-" && r.Sign() == 0)
				input := strings.Replace(string(raw), `"entryLatencyMS":0`, `"entryLatencyMS":`+tok, 1)
				cfg, err := DecodeFrozenLevelConfigJSON([]byte(input))
				if (err == nil) != want {
					t.Fatalf("oracle mismatch %s want %v got %v", tok, want, err)
				}
				if err == nil && cfg["frozenLevelBreakout"].(map[string]any)["entryLatencyMS"] != r.Num().Int64() {
					t.Fatal("value mismatch", tok)
				}
				n++
			}
		}
	}
	t.Logf("independent exact-rational integer cases: %d", n)
}

func TestReviewDuplicateEveryObjectAndEscapedKey(t *testing.T) {
	raw, e := os.ReadFile("testdata/frozen_level/config.canonical.json")
	if e != nil {
		t.Fatal(e)
	}
	var obj map[string]any
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	if e = d.Decode(&obj); e != nil {
		t.Fatal(e)
	}
	n := 0
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				key, _ := json.Marshal(k)
				scalar, _ := json.Marshal(val)
				escaped := fmt.Sprintf(`"\u%04x%s"`, k[0], k[1:])
				find := string(key) + ":" + string(scalar)
				for _, dup := range []string{string(key), escaped} {
					mutated := strings.Replace(string(raw), find, find+","+dup+":"+string(scalar), 1)
					if mutated == string(raw) {
						t.Fatal("missed replacement", k)
					}
					if cfg, e := DecodeFrozenLevelConfigJSON([]byte(mutated)); e == nil || cfg != nil {
						t.Fatal("duplicate accepted", k, dup)
					}
					n++
				}
				walk(val)
			}
		case []any:
			for _, v := range x {
				walk(v)
			}
		}
	}
	walk(obj)
	t.Logf("independent raw duplicate cases: %d", n)
}

func TestReviewEveryPermutedBlock(t *testing.T) {
	b, e := os.ReadFile("testdata/frozen_level/example.strat")
	if e != nil {
		t.Fatal(e)
	}
	s := string(b)
	names := []string{"strategy", "market conditions", "setup", "risk", "management", "execution"}
	starts := []int{}
	for _, x := range names {
		starts = append(starts, strings.Index(s, "\n"+x+" ")+1)
	}
	starts = append(starts, len(s))
	blocks := []string{}
	for i := range names {
		blocks = append(blocks, s[starts[i]:starts[i+1]])
	}
	want, _ := Parse(s)
	n := 0
	var permute func(int)
	permute = func(i int) {
		if i == len(blocks) {
			r, e := Parse("dsl v7\n" + strings.Join(blocks, ""))
			if e != nil || !reflect.DeepEqual(want, r) {
				t.Fatal("block permutation changed parse", e, r.Errors)
			}
			n++
			return
		}
		for j := i; j < len(blocks); j++ {
			blocks[i], blocks[j] = blocks[j], blocks[i]
			permute(i + 1)
			blocks[i], blocks[j] = blocks[j], blocks[i]
		}
	}
	permute(0)
	t.Logf("independent complete block permutations: %d", n)
}

func TestReviewLegacyFrozenSymbolPreserved(t *testing.T) {
	s := "dsl v7\nstrategy \"Old valid\"\nsymbols FROZEN\nsetup { type: flag continuation }\n"
	p := newParser(s)
	p.parse()
	want := p.result()
	if len(want.Errors) > 0 {
		t.Fatal("invalid legacy control", want.Errors)
	}
	got, e := Parse(s)
	if e != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy symbol changed: old errors=%v family=%v; new errors=%v family=%v", want.Errors, want.Config["setupType"], got.Errors, got.Config["setupType"])
	}
}

func TestReviewMalformedFrozenMustNotBecomeOldFamily(t *testing.T) {
	for _, s := range []string{
		"dsl v7\nstrategy \"x\"\nsetup { type: \"frozen level breakout\" }\n",
		"dsl v7\nstrategy \"x\"\ndescription \"missing end\nsetup { type: frozen level breakout type: flag continuation }\n",
	} {
		r, e := Parse(s)
		if e != nil {
			t.Fatal(e)
		}
		if len(r.Errors) == 0 || len(r.Config) != 0 {
			t.Errorf("malformed frozen became %v with errors %v", r.Config["setupType"], r.Errors)
		}
	}
}
