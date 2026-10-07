# Native adaptive flag numeric-cap research contract

Canonical task: [HT-191](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-191-adaptive-flag-cap-ablations.md).

This isolated native research overlay implements the approved definition
`adaptive-G123-cap-only-fixed-core-v1`, SHA256
`332b5037247bbd904f4e05923e286385cc6b9b53e3723297447810139982869e`.
The exact core definition was clarified after earlier A/B/C and F1–F3
observations, before G implementation or outcomes. It was not fully frozen
before all earlier observations. No historical G execution is qualified here.

## Closed interface

Only `AdaptiveFlagRequest.ResearchAblation` and the dedicated native command
accept a research request:

```
heisentick adaptive-flag-report --dsl-file=<exact-C.strat> --bars-file=<bars.btb1> \
  --research-ablation=C_RETRACE_CAP_OFF_CORE_V1
```

The optional execution window remains `--trade-from=<UTC RFC3339>` and
`--trade-to=<UTC RFC3339>`. The research request is a single exact enum:

| Candidate | Enum | Retrace cap | Width cap | EMA | Volume |
| --- | --- | --- | --- | --- | --- |
| G1 | C_RETRACE_CAP_OFF_CORE_V1 | off | on | on | on |
| G2 | C_WIDTH_CAP_OFF_CORE_V1 | on | off | on | on |
| G3 | C_RETRACE_WIDTH_CAP_OFF_CORE_V1 | off | off | on | on |

The API zero value and omitted CLI option preserve legacy behavior and bytes.
An explicitly empty CLI option rejects. Unknown/repeated/mixed requests,
CUSTOM lookalikes, INITIAL/TWEAKED, and any change to the 18 SNAPSHOT_C rules
reject. Generic/shared/prepared/prefix/checkpoint/WASM routes remain refused.
No DSL, parser, app, configuration default, portfolio rule, or release changes.

## Fixed core and overlay

Unmodified source snapshots are computed first. The overlay changes only
`BullValid`, `BearValid`, and the quote returned by the unchanged
`adaptiveFlagCandidate`, before existing finite-state validation and the full
unchanged stateful portfolio replay.

The core is exactly `BullImpulse && BullRetrace != nil` and its bear equivalent.
Both confirmed pivots remain mandatory; source origin age/order, ATR and pole
thresholds remain unchanged. A disabled cap bypasses only its numeric comparison.
Raw retrace remains null when unavailable. No positivity clamp, epsilon, ratio,
threshold substitute, numeric repair or short-side asymmetry correction is added.
Both EMA and volume remain active. Raw indicators, windows, moving endpoints,
source impulses, quote geometry and source arithmetic remain untouched.

## Wire and identity freeze

The wire/policy contract was frozen before implementation as
`G123-wire-policy-v1-before-source-implementation`, SHA256
`6997cc5c0b7c19438f542c51ccb50de119f4b3ec31fb18cb01bedf1740d8cab7`.

- Inner: `strat-adaptive-volume-flag-cap-research-v1`
- Outer: `strat-adaptive-volume-flag-cli-cap-research-v1`
- Policy: `adaptive-G123-cap-policy-v1`
- Producer contract: `adaptive-G123-native-overlay-v1`

Only research results append `researchPolicy`, `researchPolicySha256` and
`researchProducer`. The base effective config and config hash retain their
legacy meanings and cannot identify the effective research policy alone.

Policy hashing uses SHA256 of the UTF-8 compact Go `encoding/json` struct bytes,
without a newline or an embedded hash field. The exact field order is:
`contract`, `definitionId`, `definitionSha256`, `candidateId`, `request`,
`corePolicy`, `retraceCapEnabled`, `widthCapEnabled`, `emaEnabled`,
`volumeEnabled`, `baseBundle`, `baseConfigSha256`, `sourcePineSha256`,
`referenceSha256`, `numericalPolicy`, `executionSemantics`, `producerContract`.
All values are strings except the four boolean gate states.

Policy/build metadata contains only values; every call gets independent copies.
Build provenance reports runtime Go version and available VCS revision/dirty
state, without inventing a clean commit. A qualification receipt must separately
pin the exact source checkpoint and executable SHA256. A dirty build is not a
published producer identity, and the contract ID alone is not a binary digest.

Consumers must strictly admit both research schemas and the complete effective
policy, and propagate research identity separately from stable causal candidate
IDs. Old consumers fail on the new inner schema. Research consumers must reject
missing, unknown, malformed, mixed or tampered policy/config identities. Unit-risk
and cost accounting arithmetic remain unchanged.

## Qualification and next gate

Synthetic source-backed fixtures establish counterfactual admissions, stateful
occupancy divergence, pending-level freezing, delayed activated stops, expiry,
same-close rearming, gaps, hold limits, terminal pending/open/queued states,
feature invariance, numeric boundaries, source-core exclusions and causality.
Legacy mechanics/golden tests remain applicable and goldens are not regenerated.

Independent exact diff/source/test review and a root-released immutable producer
plus consumer checkpoint are required before any historical G matrix. The next
separately authorized empirical gate is exactly 18 native replays and 54 existing
RAW/BASIC/HARSH projections: G1/G2/G3 × H1/M30 × three already frozen windows.
Retain every result, use verified C compatibility, preserve the six-ablation
family, and do not add hybrids, parameter search, fresh data or funding claims.
Uncertainty execution requires its own geometry/identity adequacy gate.
