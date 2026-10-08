# Adaptive unit reporting: B0 shapes and qualification inputs

This is the B0 checkpoint of
[HT195](https://github.com/spik3r/heisentick-backlog/blob/main/tasks/HT-195-adaptive-flag-backtester-adoption.md).
It adds type/schema declarations, an executable serialized-size proof, a lossless
reference comparison specification and invented inputs. It does not implement an
economic projector, native command or WASM export. Stage A raw execution and all
legacy routes remain unchanged. No accounting capability is advertised by B0.

The proposed unit profile includes every supplied warmup row in its maximum
1,024 retained rows. Its effective window covers at most 366 UTC date labels;
definite/possible midnight lists together have at most 732 elements and copied
exposure gaps at most1,024. Longer inputs must be rejected without trimming.
This is at most 512 observed M30 bar-hours or 1,024 H1 bar-hours before warmup;
it cannot cover an arbitrary dense intraday year.

The reviewed design bound is 52,429,496 bytes under a 64 MiB encoded ceiling.
`ActualTypeBounds` derives separate allowances for orders, closed-trade copies,
actual fill costs, marks, invalid-risk rejections, daily rows, both year maps,
all exposure midnight/gap entries, fixed metadata and terminal duplicates.
The unchanged raw bound is charged once. Tests verify each term against the
reviewed allowance and serialize maximum repeated shapes, including worst-case
string escaping. These artificial maximum shapes are not valid economic ledgers.

This is not a peak-memory or browser-performance guarantee. Raw bytes, typed
raw structures, projection structures, encoded projection, combined output and
the JavaScript string may coexist in a later implementation. The result's raw
segment must retain exact original Stage A bytes; ordinary RawMessage marshaling
would compact them, so the Envelope type is only a shape declaration.

The frozen independent reference is HT184's retained `unit_projection.py`
SHA256 76c58c46f80217baf68fbe4071077b61134a78bde1b12b9ee3439a0de51eadce
with `research_common.py`
SHA256 0bbfb21c5ab6a121215ba5ec250b3f2fcbabed3a0cf357387a93fb538877bba8.
These are local reviewed research-source identities, not a fabricated published
app commit. The later G-aware extension is excluded. The oracle is not a runtime
dependency and has not been executed in B0.

The unit corpus references the unchanged 96-case Stage A invented corpus under
each of the three cost profiles. Six additional hand-built adapter ledgers and
two distinct operation-bit vectors are frozen before the Go port or oracle run.
Four additional literal compact JSON/SHA vectors freeze all three economic
policies and the numerical definition; native/WASM tests verify serializer order
and number spelling without calculating economics.
The price-derived vector and literal 1.2 gross vector deliberately differ by one
binary64 unit. Hand fixtures are adapter inputs, not strategy-generation evidence. Each declares
explicit invented outer hash sentinels and the exact byte composition/hash of a
future test-only reference wrapper. Those sentinels do not authenticate compiled
DSL/config/BTB1 and are distinct from raw-run hashes; no runtime upload is added.
Their complete canonical INITIAL config supplies comparison context only. The
honest synthetic-exit reason remains outside the public native-reason schema.
These six fixtures test the internal complete projection mapping and make no
public runtime-envelope admission or native strategy-fill claim; the 96
source-backed cases supply the later public-route qualification.

Future comparison must retain raw JSON numeric tokens until their declared field
type is known. In particular, Go float token -0 must not pass through an integer
parser that changes it to positive zero. Missing/null/false/zero remain distinct.
Every economic leaf is compared; only explicitly mapped truthful provenance
domains may differ. The generic comparator performs no accounting calculations.

Run B0 package tests with `go test ./report/adaptiveflagunit -v`. The schema and
comparison helper tests are scoped to new artifacts; they must not import or run
the retained oracle. Independent B0 PASS and a new explicit root release are
required before the projector or transports. Oracle execution itself requires a
separate authorization over the exact frozen invented corpus. No historical
replay, release, app adoption or browser qualification follows from this checkpoint.
