"""Generic token/bit and static fixture-schema probes; no economics oracle."""
import hashlib
import importlib.util
from copy import deepcopy
import json
from pathlib import Path
import struct
import sys
import unittest

MODULE = Path(__file__).with_name("compare.py")
SPEC = importlib.util.spec_from_file_location("unit_lossless_compare", MODULE)
compare = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = compare
SPEC.loader.exec_module(compare)

FLOAT = {"type": "float64"}
INTEGER = {"type": "integer", "minimum": -9007199254740991, "maximum": 9007199254740991}
REPO_ROOT = MODULE.parents[3]

# Read from dsl/adaptive_flag_config.go's INITIAL preset; not oracle output.
# This is input metadata only, with every number retained as a JSON token.
INITIAL_CONFIG_JSON = r'''{
  "policy": "DELAYED_OHLC_REFERENCE_V1",
  "numericalPolicy": "BINARY64_ORDERED_V1",
  "timeframe": "M30",
  "bundle": "INITIAL",
  "rules": {
    "pivotSensitivity": 3, "minPoleATR": 1.8,
    "minFlagBars": 3, "maxFlagBars": 16, "maxFlagRetrace": 0.50,
    "useVolumeFilter": true, "useEMATrend": true,
    "fastEMALen": 50, "slowEMALen": 200,
    "targetR": 2.5, "atrStopMult": 1.2, "validBars": 12, "maxHold": 60,
    "atrLen": 14, "volumeSMALen": 20, "volumeSMAMult": 0.9,
    "flagWidthPoleMult": 0.55, "entryBufferATR": 0.05
  }
}'''


def hand_inputs():
    data_dir = REPO_ROOT / "testsupport" / "testdata"
    corpus = json.loads((data_dir / "adaptive-flag-unit-corpus-v1.json").read_text(encoding="utf-8"))
    result = []
    for entry in corpus["handLedgerInputs"]:
        body = (data_dir / entry["file"]).read_bytes()
        result.append((entry, body, compare.loads_lossless(body)))
    return result


class LosslessComparisonTests(unittest.TestCase):
    def assertPass(self, expected, actual, schema=FLOAT):
        self.assertEqual([], compare.compare_json(expected, actual, schema))

    def assertFail(self, expected, actual, schema=FLOAT):
        result = compare.compare_json(expected, actual, schema)
        self.assertTrue(result)
        return result

    def test_both_number_callbacks_preserve_original_token(self):
        tokens = compare.loads_lossless("[-0,0,1.0,1e0,5e-324]")
        self.assertEqual(["-0", "0", "1.0", "1e0", "5e-324"], [x.text for x in tokens])

    def test_negative_zero_not_positive_zero(self):
        receipt = self.assertFail("-0", "0")[0]
        self.assertEqual("-0", receipt["expected"]["token"])
        self.assertEqual("0", receipt["actual"]["token"])
        self.assertEqual("0x8000000000000000", receipt["expected"]["binary64_bits"])
        self.assertEqual("0x0000000000000000", receipt["actual"]["binary64_bits"])
        self.assertPass("-0", "-0.0")
        self.assertPass("0", "0.0")

    def test_integer_looking_float_is_schema_typed(self):
        self.assertPass("1", "1.0")
        self.assertPass("1.0", "1e0")
        self.assertPass("1", "1", INTEGER)
        self.assertFail("1", "1.0", INTEGER)
        self.assertFail("1", "1e0", INTEGER)
        self.assertPass("-0", "0", INTEGER)

    def test_integer_value_is_exact_and_range_checked(self):
        self.assertFail("9007199254740991", "9007199254740990", INTEGER)
        self.assertFail("9007199254740992", "9007199254740992", INTEGER)
        wide = {"type": "integer"}
        self.assertFail("9007199254740993", "9007199254740992", wide)

    def test_subnormal_underflow_and_adjacent_values(self):
        self.assertPass("5e-324", "4.9406564584124654e-324")
        self.assertFail("5e-324", "0")
        self.assertFail("5e-324", "1e-323")
        self.assertPass("1e-9999", "0")
        self.assertPass("-1e-9999", "-0")
        self.assertFail("-1e-9999", "0")
        self.assertFail("1", "1.0000000000000002")
        self.assertFail("1.181", "1.1809999999999998")

    def test_independent_python_float_and_cancellation_bit_probe(self):
        # Expected constants are bit patterns, not an economic calculation.
        plus_zero = struct.unpack(">d", bytes.fromhex("0000000000000000"))[0]
        minus_zero = struct.unpack(">d", bytes.fromhex("8000000000000000"))[0]
        smallest = struct.unpack(">d", bytes.fromhex("0000000000000001"))[0]
        self.assertEqual([], compare.compare_values(plus_zero, compare.loads_lossless("0"), FLOAT))
        self.assertEqual([], compare.compare_values(minus_zero, compare.loads_lossless("-0"), FLOAT))
        self.assertEqual([], compare.compare_values(smallest, compare.loads_lossless("5e-324"), FLOAT))
        self.assertTrue(compare.compare_values(plus_zero, compare.loads_lossless("1"), FLOAT))
        # Actual ordinary decoding is forbidden, even when the value looks right.
        self.assertTrue(compare.compare_values(0.0, json.loads("0"), FLOAT))

    def test_null_missing_false_zero_and_empty_values_are_distinct(self):
        optional = {"type": "object", "properties": {"x": {"type": "float64", "nullable": True}}, "required": []}
        self.assertFail("{}", '{"x":null}', optional)
        self.assertFail('{"x":null}', '{"x":0}', optional)
        self.assertFail('{"x":false}', '{"x":0}', optional)
        self.assertPass('{"x":null}', '{"x":null}', optional)
        self.assertPass("false", "false", {"type": "boolean"})
        self.assertFail("false", "0", {"type": "boolean"})
        self.assertFail("[]", "null", {"type": "array", "items": FLOAT})
        self.assertFail("{}", "[]", {"type": "object", "properties": {}})

    def test_closed_objects_reject_unknown_even_on_both_sides(self):
        schema = {"type": "object", "properties": {"x": INTEGER}}
        self.assertFail('{"x":1}', '{"x":1,"surprise":2}', schema)
        self.assertFail('{"x":1,"surprise":2}', '{"x":1,"surprise":2}', schema)
        self.assertFail("{}", "{}", schema)
        self.assertFail('{"x":1}', "{}", schema)

    def test_presence_group_and_nullable_objects(self):
        schema = {"type": "object", "properties": {"a": FLOAT, "b": FLOAT},
                  "required": [], "presenceGroups": [["a", "b"]]}
        self.assertPass("{}", "{}", schema)
        self.assertFail('{"a":1}', '{"a":1}', schema)
        self.assertPass('{"a":1,"b":2}', '{"a":1.0,"b":2.0}', schema)
        nullable = {"type": "object", "nullable": True, "properties": {"x": INTEGER}}
        self.assertPass("null", "null", nullable)
        self.assertFail("null", '{"x":0}', nullable)

    def test_array_order_length_map_keys_and_nested_unknowns(self):
        array = {"type": "array", "items": INTEGER, "maxItems": 2}
        self.assertFail("[1,2]", "[2,1]", array)
        self.assertFail("[1]", "[1,2]", array)
        self.assertFail("[1,2,3]", "[1,2,3]", array)
        years = {"type": "object", "keyPattern": "[0-9]{4}", "values": {"type": "object", "properties": {"net": FLOAT}}}
        self.assertPass('{"2026":{"net":1}}', '{"2026":{"net":1.0}}', years)
        self.assertFail('{"2026":{"net":1}}', '{"2027":{"net":1}}', years)
        self.assertFail('{"2026":{"net":1}}', '{"2026":{"net":1,"extra":false}}', years)
        self.assertFail('{"oops":{"net":1}}', '{"oops":{"net":1}}', years)

    def test_overflow_and_nonfinite_are_rejected(self):
        self.assertFail("1e9999", "1e9999")
        self.assertFail("-1e9999", "0")
        for token in ("NaN", "Infinity", "-Infinity", "+1", "01", ".1", "1."):
            with self.subTest(token=token), self.assertRaises(compare.InvalidJSON):
                compare.loads_lossless(token)

    def test_duplicates_trailing_document_unicode(self):
        for document in ('{"x":1,"x":2}', '{"x":1,"\\u0078":2}', "{} {}", '"\\ud800"', '"\\udfff"'):
            with self.subTest(document=document), self.assertRaises(compare.InvalidJSON):
                compare.loads_lossless(document)
        with self.assertRaises(UnicodeError):
            compare.loads_lossless(b'"\xff"')
        self.assertEqual("😀", compare.loads_lossless('"\\ud83d\\ude00"'))

    def test_schema_references_and_string_enum(self):
        definitions = {"Status": {"type": "string", "enum": ["finite", "empty"]}}
        self.assertEqual([], compare.compare_json('"finite"', '"finite"', {"$ref": "#/$defs/Status"}, definitions=definitions))
        self.assertTrue(compare.compare_json('"unknown"', '"unknown"', {"$ref": "#/$defs/Status"}, definitions=definitions))

    def test_mapped_renames_preserve_bits_and_evidence(self):
        mapping = {"$defs": {},
                   "referenceRoot": {"type": "object", "properties": {"value": FLOAT, "runtime": {"type": "string"}}},
                   "goRoot": {"type": "object", "properties": {"amount": FLOAT, "build": {"type": "string"}}},
                   "fields": [
                       {"reference": "", "go": "", "type": "object", "mode": "structure"},
                       {"reference": "/value", "go": "/amount", "type": "float64", "mode": "equal"},
                       {"reference": "/runtime", "go": "/build", "type": "string", "mode": "evidence_only", "reason": "different observed runtimes"}]}
        result = compare.compare_mapped_json('{"value":-0,"runtime":"python"}', '{"amount":-0.0,"build":"go"}', mapping)
        self.assertTrue(result["pass"])
        self.assertEqual("python", result["evidence"][0]["reference"][0]["value"])
        result = compare.compare_mapped_json('{"value":-0,"runtime":"python"}', '{"amount":0,"build":"go"}', mapping)
        self.assertFalse(result["pass"])
        self.assertEqual("0x8000000000000000", result["mismatches"][0]["expected"]["binary64_bits"])
        missing = deepcopy(mapping)
        missing["fields"].pop(1)
        with self.assertRaisesRegex(ValueError, "unmapped"):
            compare.validate_mapping(missing)
        duplicate = deepcopy(mapping)
        duplicate["fields"].append(duplicate["fields"][1])
        with self.assertRaisesRegex(ValueError, "duplicate"):
            compare.validate_mapping(duplicate)
        excluded = deepcopy(mapping)
        excluded["fields"][0]["mode"] = "evidence_only"
        with self.assertRaisesRegex(ValueError, "individual leaves"):
            compare.validate_mapping(excluded)

    def test_frozen_mapping_is_complete_and_economics_are_not_excluded(self):
        mapping = json.loads(MODULE.with_name("field-mapping-v1.json").read_text(encoding="utf-8"))
        compare.validate_mapping(mapping)
        self.assertEqual(348, len(mapping["fields"]))
        for rule in mapping["fields"]:
            path = rule.get("reference")
            if path and not path.startswith("/manifest") and path != "/schema":
                self.assertIn(rule["mode"], ("equal", "structure"), path)
                self.assertEqual(path, rule["go"])
            self.assertNotIn("**", path or "")
            self.assertNotIn("**", rule.get("go") or "")
        paths = compare.schema_paths(mapping["referenceRoot"], mapping["$defs"])
        self.assertNotIn("/candidate_registry", paths)
        order = paths["/orders/*"]
        filled = mapping["profile"]["optionalFillFields"]
        self.assertEqual([filled], order["presenceGroups"])
        self.assertTrue(all(key not in order["required"] for key in filled))
        for path in ("/closed_trades/*", "/terminal/open_position"):
            self.assertTrue(all(key in paths[path]["required"] for key in filled))

    def test_forged_number_tokens_do_not_bypass_json_syntax(self):
        self.assertTrue(compare.compare_values(1.0, compare.NumberToken("+1"), FLOAT))
        self.assertTrue(compare.compare_values(1.0, compare.NumberToken("01"), FLOAT))

    def test_all_six_hand_configs_match_complete_canonical_initial_mapping(self):
        mapping = json.loads(MODULE.with_name("field-mapping-v1.json").read_text(encoding="utf-8"))
        definitions = mapping["$defs"]
        schema = {"$ref": "#/$defs/AdaptiveFlagSpec"}
        compare.validate_mapping(mapping)
        expected = compare.loads_lossless(INITIAL_CONFIG_JSON)
        inputs = hand_inputs()
        self.assertEqual({"hand-long-closed", "hand-short-closed", "hand-long-open",
                          "hand-long-pending", "hand-invalid-risk-pending", "hand-invalid-risk-filled"},
                         {entry["id"] for entry, _, _ in inputs})
        self.assertEqual(6, len(inputs))
        by_reference = {entry["reference"]: entry for entry in mapping["fields"]
                        if entry.get("reference") is not None}
        for path, spec in compare.schema_paths(schema, definitions).items():
            rule = by_reference["/manifest/effective_config" + path]
            self.assertEqual("/manifest/effectiveConfig" + path, rule["go"])
            self.assertEqual(spec["type"], rule["type"])
            self.assertEqual("structure" if spec["type"] == "object" else "equal", rule["mode"])
        for entry, body, raw in inputs:
            with self.subTest(fixture=entry["id"]):
                self.assertEqual(entry["sha256"], hashlib.sha256(body).hexdigest())
                self.assertIs(entry["oracleExecuted"], False)
                self.assertEqual([], compare.compare_values(expected, raw["effectiveConfig"], schema,
                                                            definitions=definitions))

    def test_incomplete_hand_config_and_each_missing_leaf_are_rejected(self):
        mapping = json.loads(MODULE.with_name("field-mapping-v1.json").read_text(encoding="utf-8"))
        definitions = mapping["$defs"]
        schema = {"$ref": "#/$defs/AdaptiveFlagSpec"}
        expected = compare.loads_lossless(INITIAL_CONFIG_JSON)
        incomplete = compare.loads_lossless('{"timeframe":"M30","bundle":"INITIAL","rules":{}}')
        self.assertTrue(compare.compare_values(expected, incomplete, schema, definitions=definitions))
        for entry, _, raw in hand_inputs():
            for parent, keys in (((), tuple(expected)), (("rules",), tuple(expected["rules"]))):
                for key in keys:
                    with self.subTest(fixture=entry["id"], parent=parent, missing=key):
                        candidate = deepcopy(raw["effectiveConfig"])
                        target = candidate["rules"] if parent else candidate
                        del target[key]
                        self.assertTrue(compare.compare_values(expected, candidate, schema,
                                                               definitions=definitions))

    def test_synthetic_exit_labels_remain_honest_adapter_only_inputs(self):
        # These reasons are intentionally invented adapter evidence. Never
        # rewrite them into plausible native exit labels to satisfy a schema.
        # No public runtime-envelope or strategy-generation claim follows.
        expected_ids = {"hand-long-closed", "hand-short-closed", "hand-invalid-risk-filled"}
        observed = set()
        for entry, _, raw in hand_inputs():
            self.assertEqual("internal complete projection mapping; adapter-only invented input",
                             entry["comparisonDomain"])
            self.assertIs(entry["publicRuntimeEnvelopeAdmissionClaim"], False)
            self.assertIs(entry["nativeStrategyFillEvidence"], False)
            if entry["id"] in expected_ids:
                self.assertEqual(1, len(raw["orders"]))
                self.assertEqual("synthetic-exit", raw["orders"][0]["reason"])
                self.assertEqual(1, sum(event["state"] == "closed" for event in raw["events"]))
                observed.add(entry["id"])
            else:
                self.assertTrue(all(order["reason"] != "synthetic-exit" for order in raw["orders"]))
            self.assertTrue(all(event["basis"] == "invented adapter fixture" for event in raw["events"]))
            self.assertEqual("invented test-only sentinels, not authenticated compiled DSL/config/BTB1 hashes",
                             entry["referenceInputWrapperIdentityKind"])
            self.assertIs(entry["oracleExecuted"], False)
        self.assertEqual(expected_ids, observed)


if __name__ == "__main__":
    unittest.main()
