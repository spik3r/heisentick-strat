// Command harness runs the engine's public API over a case manifest and writes
// one JSON receipt line per case. Every float is recorded as its IEEE-754 bit
// pattern after the public API rounds numbers to 15 significant decimal digits.
// This does not expose raw broker intermediates or level history. It reads engine source only
// through the exported API and changes nothing in the engine.
//
//	harness -root <strat checkout> -manifest <file> [-case <id>] > receipts.jsonl
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/marketdata"
)

type costOverride struct {
	FeePerUnit  *string `json:"feePerUnit,omitempty"` // exact decimal text, parsed as binary64
	Slippage    *string `json:"slippage,omitempty"`
	SlippageBps *string `json:"slippageBps,omitempty"`
	StartEquity *string `json:"startEquity,omitempty"`
}

type barPatch struct {
	Index int     `json:"index"`
	O     *string `json:"o,omitempty"`
	H     *string `json:"h,omitempty"`
	L     *string `json:"l,omitempty"`
	C     *string `json:"c,omitempty"`
}

type manifestCase struct {
	Scale   *string       `json:"scale,omitempty"` // exact decimal multiplier for every O/H/L/C
	Patches []barPatch    `json:"patches,omitempty"`
	ID      string        `json:"id"`
	Source  string        `json:"source"` // path of <source>.fixture.json/.strat relative to root
	Costs   *costOverride `json:"costs,omitempty"`
	Note    string        `json:"note,omitempty"`
}

type manifest struct {
	Schema string         `json:"schema"`
	Cases  []manifestCase `json:"cases"`
}

type tradeRecord struct {
	Side       string `json:"side"`
	EntryIndex int    `json:"entryIndex"`
	ExitIndex  int    `json:"exitIndex"`
	Reason     string `json:"reason"`
	Rule       string `json:"rule,omitempty"`
	Tag        string `json:"tag,omitempty"`
	Partial    bool   `json:"partial,omitempty"`
	// Price path and sizing, as float64 bit patterns.
	Entry  string `json:"entry"`
	Exit   string `json:"exit"`
	SL     string `json:"sl"`
	InitSL string `json:"initialSl"`
	InitTP string `json:"initialTp"`
	TP     string `json:"tp"`
	Size   string `json:"size"`
	Points string `json:"points"`
	// Reported amount.
	PnL string `json:"pnl"`
	// Decimal text of pnl for human reading; not used for comparison.
	PnLText string `json:"pnlText"`
}

type receipt struct {
	Schema       string        `json:"schema"`
	ID           string        `json:"id"`
	Target       string        `json:"target"`
	InputSHA256  string        `json:"inputSha256"`
	CostBits     costBits      `json:"costBits"`
	Error        string        `json:"error,omitempty"`
	TradeCount   int           `json:"tradeCount"`
	DecisionHash string        `json:"decisionHash"` // sides, indices, reasons, rules, tags
	PathHash     string        `json:"pathHash"`     // decision + entry/exit/sl/tp/size/points bits
	PnLHash      string        `json:"pnlHash"`
	Trades       []tradeRecord `json:"trades"`
}

type costBits struct {
	FeePerUnit  string `json:"feePerUnit"`
	Slippage    string `json:"slippage"`
	SlippageBps string `json:"slippageBps"`
	StartEquity string `json:"startEquity"`
	FillOn      string `json:"fillOn"`
}

var buildSourceSHA string // injected by the evidence runner into this binary

type targetInfo struct {
	Settings     map[string]string `json:"settings"`
	SourceSHA256 string            `json:"sourceSha256"`
	Go           string            `json:"go"`
	GOOS         string            `json:"goos"`
	GOARCH       string            `json:"goarch"`
	GOAMD64      string            `json:"goamd64,omitempty"`
	Module       string            `json:"module"`
	VCS          string            `json:"vcsRevision,omitempty"`
	Dirty        string            `json:"vcsModified,omitempty"`
}

func bits(v float64) string { return fmt.Sprintf("%016x", math.Float64bits(v)) }

func target() targetInfo {
	info := targetInfo{Settings: map[string]string{}, SourceSHA256: buildSourceSHA, Go: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	if bi, ok := debug.ReadBuildInfo(); ok {
		info.Module = bi.Main.Path
		for _, s := range bi.Settings {
			info.Settings[s.Key] = s.Value
			switch s.Key {
			case "GOAMD64":
				info.GOAMD64 = s.Value
			case "vcs.revision":
				info.VCS = s.Value
			case "vcs.modified":
				info.Dirty = s.Value
			}
		}
	}
	return info
}

func hashStrings(parts []string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func parseOverride(text *string, into *float64) error {
	if text == nil {
		return nil
	}
	v, err := strconv.ParseFloat(*text, 64)
	if err != nil {
		return fmt.Errorf("parse %q: %w", *text, err)
	}
	*into = v
	return nil
}

func main() {
	root := flag.String("root", ".", "strat checkout root")
	manifestPath := flag.String("manifest", "", "case manifest JSON")
	only := flag.String("case", "", "run one case id")
	label := flag.String("label", "", "free-form target label recorded on every receipt")
	identity := flag.Bool("identity", false, "print executed binary runtime identity and exit")
	flag.Parse()
	if *identity {
		if err := json.NewEncoder(os.Stdout).Encode(target()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	raw, err := os.ReadFile(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	info, _ := json.Marshal(target())
	tgt := string(info)
	if *label != "" {
		tgt = *label + " " + tgt
	}
	enc := json.NewEncoder(os.Stdout)
	for _, c := range m.Cases {
		if *only != "" && c.ID != *only {
			continue
		}
		r := runCase(*root, c)
		r.Target = tgt
		if err := enc.Encode(r); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func runCase(root string, c manifestCase) receipt {
	r := receipt{Schema: "ht245.receipt.v1", ID: c.ID}
	fixturePath := filepath.Join(root, c.Source+".fixture.json")
	stratPath := filepath.Join(root, c.Source+".strat")
	fx, err := os.ReadFile(fixturePath)
	if err == nil {
		var st []byte
		st, err = os.ReadFile(stratPath)
		if err == nil {
			sum := sha256.New()
			sum.Write(fx)
			sum.Write([]byte{0})
			sum.Write(st)
			if c.Costs != nil || c.Scale != nil || len(c.Patches) > 0 {
				cj, _ := json.Marshal(struct {
					C *costOverride
					S *string
					P []barPatch
				}{c.Costs, c.Scale, c.Patches})
				sum.Write([]byte{0})
				sum.Write(cj)
			}
			r.InputSHA256 = hex.EncodeToString(sum.Sum(nil))
			var fixture engine.RunFixture
			fixture, err = engine.LoadRunFixture(fixturePath)
			if err == nil {
				if c.Costs != nil {
					if err = parseOverride(c.Costs.FeePerUnit, &fixture.Costs.FeePerUnit); err == nil {
						if err = parseOverride(c.Costs.Slippage, &fixture.Costs.Slippage); err == nil {
							if err = parseOverride(c.Costs.SlippageBps, &fixture.Costs.SlippageBps); err == nil {
								err = parseOverride(c.Costs.StartEquity, &fixture.Costs.StartEquity)
							}
						}
					}
				}
				if err == nil {
					err = mutateBars(&fixture, c)
				}
				r.CostBits = costBits{bits(fixture.Costs.FeePerUnit), bits(fixture.Costs.Slippage),
					bits(fixture.Costs.SlippageBps), bits(fixture.Costs.StartEquity), fixture.Costs.FillOn}
				if err == nil {
					var result engine.RunResult
					result, err = engine.RunFixtureCase(fixture, string(st))
					if err == nil {
						fill(&r, result.Trades)
					}
				}
			}
		}
	}
	if err != nil {
		r.Error = strings.ReplaceAll(err.Error(), root, "<root>")
	}
	return r
}

func fill(r *receipt, trades []engine.Trade) {
	var decision, path, pnl []string
	for _, t := range trades {
		rec := tradeRecord{Side: t.Side, EntryIndex: t.EntryIndex, ExitIndex: t.ExitIndex, Reason: t.Reason,
			Rule: t.Rule, Tag: t.Tag, Partial: t.Partial,
			Entry: bits(t.Entry), Exit: bits(t.Exit), SL: bits(t.SL), TP: bits(t.TP), InitSL: bits(t.InitialSL), InitTP: bits(t.InitialTP), Size: bits(t.Size),
			Points: bits(t.Points), PnL: bits(t.PnL), PnLText: fmt.Sprintf("%.17g", t.PnL)}
		r.Trades = append(r.Trades, rec)
		d := fmt.Sprintf("%s|%d|%d|%s|%s|%s|%t", rec.Side, rec.EntryIndex, rec.ExitIndex, rec.Reason, rec.Rule, rec.Tag, rec.Partial)
		decision = append(decision, d)
		path = append(path, d+"|"+rec.Entry+rec.Exit+rec.SL+rec.TP+rec.Size+rec.Points)
		pnl = append(pnl, rec.PnL)
	}
	r.TradeCount = len(trades)
	r.DecisionHash = hashStrings(decision)
	r.PathHash = hashStrings(path)
	r.PnLHash = hashStrings(pnl)
}

func mutateBars(f *engine.RunFixture, c manifestCase) error {
	if c.Scale != nil {
		k, err := strconv.ParseFloat(*c.Scale, 64)
		if err != nil {
			return err
		}
		for _, bars := range [][]marketdata.Bar{f.Bars, f.SourceBars, f.HTFBars, f.SourceHTFBars} {
			for i := range bars {
				bars[i].O *= k
				bars[i].H *= k
				bars[i].L *= k
				bars[i].C *= k
			}
		}
	}
	for _, p := range c.Patches {
		if p.Index < 0 || p.Index >= len(f.Bars) {
			return fmt.Errorf("patch index %d out of range", p.Index)
		}
		for _, one := range []struct {
			text *string
			into *float64
		}{{p.O, &f.Bars[p.Index].O}, {p.H, &f.Bars[p.Index].H}, {p.L, &f.Bars[p.Index].L}, {p.C, &f.Bars[p.Index].C}} {
			if err := parseOverride(one.text, one.into); err != nil {
				return err
			}
		}
	}
	return nil
}
