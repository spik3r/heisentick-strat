package engine

import (
	"github.com/spik3r/heisentick-strat/dsl"
	"math"
	"reflect"
	"regexp"
	"strings"
)

// This is an authored-capability audit, not a strategy-source allowlist. Every
// occurrence is checked so a later reset cannot hide an unsupported directive.
func bidAskAdmitDualEMA(source string, cfg dsl.Config, route PaperRoute) error {
	audit := dsl.AuditOrdinarySource(source)
	if len(audit.Issues) != 0 {
		return bidAskError("ADMISSION")
	}
	n := `(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)`
	form := regexp.MustCompile(`^(?:dsl v7|type dual ema resumption|ema (?:fast|slow|slow rise|wilder atr) ` + n + `|stop initial ` + n + ` atr|trail close ` + n + ` atr|fallback below slow ema|side long only|risk ` + n + ` usd|slices xauusd 4h)$`)
	seen := false
	for _, directive := range audit.Directives {
		tokens := directive.Tokens
		text := strings.Join(tokens, " ")
		head := strings.ToLower(tokens[0])
		if head == "strategy" || head == "name" || head == "description" {
			if !paperORBMetadata.MatchString(text) {
				return bidAskError("ADMISSION")
			}
			continue
		}
		expected := map[string]string{"dsl": "", "slices": "market", "type": "setup", "ema": "setup", "stop": "setup", "trail": "setup", "fallback": "setup", "side": "filters", "risk": "execution"}
		section, known := expected[head]
		if !known || directive.Section != section {
			return bidAskError("ADMISSION")
		}
		lower := strings.ToLower(text)
		if !form.MatchString(lower) {
			return bidAskError("ADMISSION")
		}
		for _, token := range tokens {
			if !paperORBNumberToken.MatchString(strings.ToLower(token)) {
				continue
			}
			value := paperORBTokenNumber(token)
			if !isFinite(value) || value <= 0 || (head == "ema" && (math.Trunc(value) != value || value >= math.Exp2(31))) {
				return bidAskError("ADMISSION")
			}
		}
		if lower == "type dual ema resumption" {
			seen = true
		}
	}
	if !seen || route.Symbol != "XAUUSD" || route.Timeframe != "4h" || route.RangeMethod != "zone" || !compiledRouteAllowed(cfg, route.Symbol, route.Timeframe) {
		return bidAskError("ROUTE_UNSUPPORTED")
	}
	// Compare all other leaves to the parser's defaults: no ignored filters,
	// generic management, context inputs or hidden effective options are admitted.
	neutral, err := dsl.Parse("dsl v7\nsetup { type: dual ema resumption }\nfilters { side long only }\n")
	if err != nil || len(neutral.Errors) != 0 {
		return bidAskError("ADMISSION")
	}
	for key, value := range cfg {
		switch key {
		case "name", "description", "symbols", "timeframes", "slices", "dualEmaResumption", "riskUsd":
			continue
		}
		if !reflect.DeepEqual(value, neutral.Config[key]) {
			return bidAskError("ADMISSION")
		}
	}
	p := paramsFromConfig(cfg).DualEMA
	if p.FastLen < 1 || p.SlowLen < 1 || p.RiseBars < 1 || p.ATRLen < 1 || !isFinite(p.StopATR) || p.StopATR <= 0 || !isFinite(p.TrailATR) || p.TrailATR <= 0 {
		return bidAskError("ADMISSION")
	}
	return nil
}

// Advance family state exactly once on each closed midpoint bar. Entry orders
// are captured before broker execution; warm-up discards orders and exits.
func (x *bidAskRunner) dualEMABar(i int, actionable bool) *order {
	b := &x.prepared.broker
	b.onDualEMAResumptionBar(i)
	var captured *order
	if actionable && len(b.pendingOrders) == 1 {
		value := b.pendingOrders[0]
		captured = &value
	}
	if len(b.pendingOrders) > 1 {
		x.err = bidAskError("MULTIPLE_DECISIONS")
	}
	b.pendingOrders = b.pendingOrders[:0]
	if !actionable {
		b.pendingExits = b.pendingExits[:0]
		b.dualExitPending = false
	}
	return captured
}
