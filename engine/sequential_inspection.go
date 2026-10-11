package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"runtime"
	"runtime/debug"
	"unicode/utf8"

	"github.com/spik3r/heisentick-strat/dsl"
)

const (
	SequentialInspectionSchema         = "strat-sequential-source-capability-v1"
	SequentialInspectionMaxSource      = 8 << 20
	SequentialInspectionConfigEncoding = "go-encoding-json-config-v1"
)

// InspectionFact describes producer code capability, never request admission.
// Unknown facts cannot be promoted to support by a successful parse.
type InspectionFact struct {
	Status      string   `json:"status"`
	Code        string   `json:"code"`
	Constraints []string `json:"constraints"`
}

type SequentialInspectionCapabilities struct {
	SourceParsed           InspectionFact `json:"sourceParsed"`
	EngineSupported        InspectionFact `json:"engineSupported"`
	GenericSourceExecution InspectionFact `json:"genericSourceExecution"`
	SourceAccounting       InspectionFact `json:"sourceAccounting"`
	FixtureAccounting      InspectionFact `json:"fixtureAccounting"`
	CalendarExecution      InspectionFact `json:"calendarExecution"`
	PreserveOpenEndpoint   InspectionFact `json:"preserveOpenEndpoint"`
	PreservedPrefix        InspectionFact `json:"preservedPrefix"`
	CheckpointResume       InspectionFact `json:"checkpointResume"`
	Recovery               InspectionFact `json:"recovery"`
	MarketDataReadiness    InspectionFact `json:"marketDataReadiness"`
	ConsumerIntegration    InspectionFact `json:"consumerIntegration"`
}

type SequentialInspectionSource struct {
	SHA256    string `json:"sha256"`
	UTF8Bytes int    `json:"utf8Bytes"`
}

type SequentialInspectionConfigIdentity struct {
	SHA256   string `json:"sha256"`
	Encoding string `json:"encoding"`
}

type SequentialInspectionClassification struct {
	Family    string `json:"family"`
	Profile   string `json:"profile,omitempty"`
	Policy    string `json:"policy,omitempty"`
	Symbol    string `json:"symbol,omitempty"`
	Timeframe string `json:"timeframe,omitempty"`
}

// These are available compiler stamps, not a release or artifact attestation.
// VCS fields are null when the toolchain did not embed them.
type SequentialInspectionProducer struct {
	GoVersion     string  `json:"goVersion"`
	GOOS          string  `json:"goos"`
	GOARCH        string  `json:"goarch"`
	ModuleVersion string  `json:"moduleVersion"`
	VCSRevision   *string `json:"vcsRevision"`
	VCSModified   *bool   `json:"vcsModified"`
}

type SequentialInspectionCosts struct {
	FillOn                 []string `json:"fillOn"`
	NumericDomain          string   `json:"numericDomain"`
	ZeroStartEquityDefault int      `json:"zeroStartEquityDefault"`
	FeeBasis               string   `json:"feeBasis"`
	SlippageBasis          string   `json:"slippageBasis"`
	TradePnL               string   `json:"tradePnl"`
	UnitModel              string   `json:"unitModel"`
}

type SequentialInspectionExecution struct {
	Contract string                    `json:"contract"`
	Surfaces []string                  `json:"surfaces"`
	Costs    SequentialInspectionCosts `json:"costs"`
}

type SequentialInspectionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SequentialSourceInspection is an additive, read-only producer fact envelope.
// Result is the original Go ParseResult; no second config normalization exists.
type SequentialSourceInspection struct {
	Schema                   string                              `json:"schema"`
	ContractVersion          int                                 `json:"contractVersion"`
	APIVersion               int                                 `json:"apiVersion"`
	GrammarHash              string                              `json:"grammarHash"`
	OK                       bool                                `json:"ok"`
	Source                   *SequentialInspectionSource         `json:"source,omitempty"`
	Result                   *dsl.ParseResult                    `json:"result,omitempty"`
	ConfigIdentity           *SequentialInspectionConfigIdentity `json:"configIdentity,omitempty"`
	Classification           *SequentialInspectionClassification `json:"classification,omitempty"`
	Producer                 SequentialInspectionProducer        `json:"producer"`
	Capabilities             *SequentialInspectionCapabilities   `json:"capabilities,omitempty"`
	GenericExecutionContract *SequentialInspectionExecution      `json:"genericExecutionContract,omitempty"`
	Error                    *SequentialInspectionError          `json:"error,omitempty"`
}

func inspectionFact(status, code string, constraints ...string) InspectionFact {
	return InspectionFact{status, code, append([]string{}, constraints...)}
}

func sequentialInspectionProducer() SequentialInspectionProducer {
	p := SequentialInspectionProducer{GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, ModuleVersion: "unknown"}
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" {
			p.ModuleVersion = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				if setting.Value != "" {
					value := setting.Value
					p.VCSRevision = &value
				}
			case "vcs.modified":
				if setting.Value == "true" || setting.Value == "false" {
					value := setting.Value == "true"
					p.VCSModified = &value
				}
			}
		}
	}
	return p
}

func sequentialInspectionEnvelope() SequentialSourceInspection {
	return SequentialSourceInspection{Schema: SequentialInspectionSchema, ContractVersion: 1, APIVersion: 1, GrammarHash: dsl.GrammarManifestSHA256, Producer: sequentialInspectionProducer()}
}

// SequentialInspectionFailure returns only a fixed safe transport failure.
// Unknown codes map to an internal failure and cannot supply authored facts.
func SequentialInspectionFailure(code string) SequentialSourceInspection {
	messages := map[string]string{
		"SEQUENTIAL_INSPECTION_ARGUMENTS":     "Expected exactly one source string.",
		"SEQUENTIAL_INSPECTION_SOURCE":        "Source must be nonempty valid Unicode and at most 8 MiB of UTF-8.",
		"SEQUENTIAL_INSPECTION_PARSE":         "The Go parser could not inspect this source.",
		"SEQUENTIAL_INSPECTION_SERIALIZATION": "The Go inspection result could not be serialized.",
		"SEQUENTIAL_INSPECTION_INPUT":         "The source input could not be read.",
	}
	message, ok := messages[code]
	if !ok {
		code, message = "SEQUENTIAL_INSPECTION_INTERNAL", "The Go source inspection failed."
	}
	r := sequentialInspectionEnvelope()
	r.Error = &SequentialInspectionError{code, message}
	return r
}

func unknownSequentialInspectionCapabilities(code string) *SequentialInspectionCapabilities {
	return &SequentialInspectionCapabilities{
		SourceParsed: inspectionFact("unknown", code), EngineSupported: inspectionFact("unknown", code),
		GenericSourceExecution: inspectionFact("unknown", code), SourceAccounting: inspectionFact("unknown", code),
		FixtureAccounting: inspectionFact("unknown", code), CalendarExecution: inspectionFact("unknown", code),
		PreserveOpenEndpoint: inspectionFact("unknown", code), PreservedPrefix: inspectionFact("unknown", code),
		CheckpointResume: inspectionFact("unknown", code), Recovery: inspectionFact("unknown", code),
		MarketDataReadiness: inspectionFact("unknown", "INPUTS_NOT_INSPECTED"), ConsumerIntegration: inspectionFact("unknown", "CONSUMER_NOT_INSPECTED"),
	}
}

// InspectSequentialSource parses exactly the supplied source. It runs no
// strategy, reads no bars and makes no consumer or market readiness assertion.
func InspectSequentialSource(source string) SequentialSourceInspection {
	if len(source) == 0 || len(source) > SequentialInspectionMaxSource || !utf8.ValidString(source) {
		return SequentialInspectionFailure("SEQUENTIAL_INSPECTION_SOURCE")
	}
	parsed, err := dsl.Parse(source)
	if err != nil {
		return SequentialInspectionFailure("SEQUENTIAL_INSPECTION_PARSE")
	}
	r := sequentialInspectionEnvelope()
	r.OK, r.Result = true, &parsed
	hash := sha256.Sum256([]byte(source))
	r.Source = &SequentialInspectionSource{hex.EncodeToString(hash[:]), len(source)}
	r.Capabilities = unknownSequentialInspectionCapabilities("SOURCE_PARSE_FAILED")
	if len(parsed.Errors) != 0 {
		r.Capabilities.SourceParsed = inspectionFact("unsupported", "SOURCE_PARSE_FAILED")
		return r
	}
	config, err := json.Marshal(parsed.Config)
	if err != nil {
		return SequentialInspectionFailure("SEQUENTIAL_INSPECTION_SERIALIZATION")
	}
	hash = sha256.Sum256(config)
	r.ConfigIdentity = &SequentialInspectionConfigIdentity{hex.EncodeToString(hash[:]), SequentialInspectionConfigEncoding}
	r.Capabilities = unknownSequentialInspectionCapabilities("OUTSIDE_INSPECTOR_SCOPE")
	r.Capabilities.SourceParsed = inspectionFact("supported", "SOURCE_PARSED")
	family, _ := parsed.Config["setupType"].(string)
	if id, ok := parsed.Config["setupType"].(dsl.FamilyID); ok {
		family = string(id)
	}
	if family != "" {
		r.Classification = &SequentialInspectionClassification{Family: family}
	}
	if family != string(dsl.FamilySequentialFull) {
		return r
	}
	spec, err := dsl.DecodeSequentialFullConfig(parsed.Config)
	if err != nil {
		r.Capabilities = unknownSequentialInspectionCapabilities("SEQUENTIAL_CONFIG_REJECTED")
		r.Capabilities.SourceParsed = inspectionFact("supported", "SOURCE_PARSED")
		r.Capabilities.EngineSupported = inspectionFact("unsupported", "SEQUENTIAL_CONFIG_REJECTED")
		return r
	}
	r.Classification = &SequentialInspectionClassification{family, spec.Profile, spec.Policy, spec.Symbol, spec.Timeframe}
	c := r.Capabilities
	c.EngineSupported = inspectionFact("supported", "SEQUENTIAL_FULL_CONFIG_SUPPORTED", "request-admission-required")
	c.GenericSourceExecution = inspectionFact("supported", "GENERIC_SYNTHETIC_EXECUTION_ONLY",
		"ordered-contiguous-valid-ohlc", "no-calendar", "no-source-or-htf-input", "no-execution-window-or-force-route",
		"declared-source-route-only", "retained-generic-cost-contract", "terminal-held-position-rejected", "terminal-pending-no-next-bar")
	c.SourceAccounting = inspectionFact("unsupported", "SOURCE_ACCOUNTING_EXPORT_UNAVAILABLE")
	c.FixtureAccounting = inspectionFact("unknown", "EXACT_FROZEN_FIXTURE_REQUIRED", "four-source-v1-companion", "fixture-admission-required")
	c.CalendarExecution = inspectionFact("unsupported", "SEQUENTIAL_CALENDAR_UNSUPPORTED")
	c.PreserveOpenEndpoint = inspectionFact("unsupported", "SEQUENTIAL_OPEN_ENDPOINT_UNSUPPORTED")
	c.PreservedPrefix = inspectionFact("unsupported", "SEQUENTIAL_PREFIX_UNSUPPORTED")
	c.CheckpointResume = inspectionFact("unsupported", "SEQUENTIAL_CHECKPOINT_UNSUPPORTED")
	c.Recovery = inspectionFact("unknown", "SEQUENTIAL_RECOVERY_UNQUALIFIED")
	r.GenericExecutionContract = &SequentialInspectionExecution{
		Contract: sequentialFullAuditSchema,
		Surfaces: []string{"engine.Run", "engine.PrepareRun", "engine.RunFixtureCase", "engineRunFixture"},
		Costs: SequentialInspectionCosts{[]string{"open", "nextOpen"}, "finite-nonnegative", 10000,
			"per-unit-per-side", "absolute-plus-abs-reference-price-bps", "exit-credit-excludes-entry-fee", "abstract-linear-units"},
	}
	return r
}
