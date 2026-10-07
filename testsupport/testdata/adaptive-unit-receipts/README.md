# Immutable B2 qualification receipt references

These files preserve the approved invented-data B2 receipts byte for byte:

| File | SHA256 |
| --- | --- |
| `b2-parity.json` | `b981ed430fcad8318a444d06fb5eb7684956e2b14de115c248672c93cbf3e91a` |
| `b2-transport.json` | `ed1fd53bad0af48dbbb2800f9288b7281ad3683f0d4f69726d7d3ed351f1c968` |

They are immutable reference fixtures, not evidence that the current checkout
has executed. No private filesystem paths, market data, regenerated expectations,
or Python oracle execution is included. Their recorded original build identity
is deliberately retained and is not a required identity for a new CI build.

The unit receipt validator pins these bytes and uses the complete ordered 288
report descriptors and 27 accepted / 168 refused transport descriptors. Repeated
parameterized labels remain repeated; counts alone cannot establish coverage.
Nonidentity report hashes and original core/oracle/plan identities are frozen.
Fresh native/WASM hashes, seven build identity leaves and corresponding byte
length differences are checked against the actual artifacts and host. Every
saved public report is read, hashed and compared after only those seven token
values are replaced, preserving all other original bytes, including signed zero.

Current qualification requires parity v2 with the pinned self-contained bundle
identity and transport v2 with each ordered recovery result, both complete generic
refusal results and the final recovery. Tests explicitly construct this evidence
schema around the v1 fixtures; the v1 fixtures themselves are never rewritten.
Those tests do not claim a fresh engine run. Actual integration must produce new
v2 receipts and validate their accompanying artifacts and all 576 report files.
