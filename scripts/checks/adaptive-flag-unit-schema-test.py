#!/usr/bin/env python3
"""B0-only closed-schema/source-shape tests; Python standard library only.

No economics, oracle import/execution, source-bar input, strategy run, transport,
or external schema fetch occurs. The small validator below implements only the
keywords used by these local schemas. It is a test helper, not a production JSON
admission implementation or a general JSON Schema conformance implementation.
The x-* annotations document runtime duties and are deliberately NOT validators.
"""
from __future__ import annotations

import copy
import hashlib
import itertools
import json
import math
from pathlib import Path
import re
import sys
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCHEMAS = ROOT / "spec/schemas"
PREFIX = "adaptive-flag-unit-"
FILES = {role: PREFIX + role + "-v1.schema.json" for role in ("request", "envelope", "error")}
N, D, E, G = 1024, 366, 732, 1024
FILL = ("raw_entry", "fill_risk_raw_U", "display_proxy_adjusted_entry", "fill_risk_effective_U")
PAIRS = {
    "raw_request_rejected": "request", "projection_request_rejected": "request",
    "raw_source_rejected": "source", "raw_input_rejected": "input",
    "projection_domain_rejected": "input", "resource_limit": "resource",
    "raw_execution_rejected": "execution", "internal_error": "execution",
    "invalid_native_ledger": "projection", "invalid_planned_risk": "projection",
    "nonfinite_projection_arithmetic": "projection",
    "ledger_reconciliation_failed": "projection", "daily_reconciliation_failed": "projection",
}
OPERATIONS = (
    "raw_request projection_request source btb1_header btb1_copy raw_build raw_decode "
    "time_domain calendar_count output_bound exposure_count lifecycle distance quantity "
    "entry_charge exit_charge cost_component fill_risk_raw display_adjustment "
    "fill_risk_effective gross gross_realized paid_cost net closed_net open_gross marked "
    "alternate reconciliation_delta close_drawdown closed_drawdown summary_positive "
    "summary_negative summary_net summary_mean summary_pf daily_change "
    "daily_reconciliation calendar_year_sum hypothetical_exit_cost "
    "hypothetical_liquidation identity serialize transport native_io"
).split()
STAGE_A_SHA256 = {
    "adaptive-flag-runtime-envelope-v1.schema.json": "aa01920af0bd0bd506269ad4514d8096b6e6a492a0c6479ae66b62e69998cb37",
    "adaptive-flag-runtime-request-v1.schema.json": "75bdaea2a58cc616abc0c67658b231095685460f088edbc09490dd904cc2eb98",
    "adaptive-flag-raw-run-v1.schema.json": "66988f25b020f6b1793789b0c14ca1cc885b1cd062bf2748592c995a3fde8ab1",
    "adaptive-volume-flag-config-v1.schema.json": "3ae2192b2debb69457fe333e6ebb791dd1cdd40c3782d0ad4933bd67c21098d9",
    "adaptive-flag-runtime-error-v1.schema.json": "ca262fee9b12b4c380287f1acb220225207f8a463e8364cff06dafc71d7d62fb",
}


class Invalid(ValueError):
    pass


def strict_load(text):
    def object_pairs(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise Invalid("duplicate decoded object key")
            result[key] = value
        return result

    def nonjson(token):
        raise Invalid("non-JSON numeric token: " + token)

    return json.loads(text, object_pairs_hook=object_pairs, parse_constant=nonjson)


def same(a, b):
    # JSON number equality allows 1 == 1.0 but never false == 0.
    if isinstance(a, bool) or isinstance(b, bool):
        return type(a) is type(b) and a == b
    return a == b


class LocalSchemas:
    def __init__(self):
        self.docs = {}

    def load(self, name):
        if Path(name).name != name or not name.endswith(".schema.json"):
            raise Invalid("only local sibling schema files are supported")
        if name not in self.docs:
            self.docs[name] = strict_load((SCHEMAS / name).read_text(encoding="utf-8"))
        return self.docs[name]

    def resolve(self, reference, filename):
        other, _, pointer = reference.partition("#")
        filename = other or filename
        node = self.load(filename)
        if pointer:
            if not pointer.startswith("/"):
                raise Invalid("only JSON Pointer fragments are supported")
            for part in pointer[1:].split("/"):
                node = node[part.replace("~1", "/").replace("~0", "~")]
        return node, filename

    def valid(self, value, schema, filename):
        try:
            self.validate(value, schema, filename)
            return True
        except Invalid:
            return False

    def validate(self, value, schema, filename):
        def require(condition, label):
            if not condition:
                raise Invalid(label)

        if schema is True:
            return
        if schema is False:
            raise Invalid("false schema")
        if "$ref" in schema:
            target, target_file = self.resolve(schema["$ref"], filename)
            self.validate(value, target, target_file)
        if "const" in schema:
            require(same(value, schema["const"]), "const")
        if "enum" in schema:
            require(any(same(value, item) for item in schema["enum"]), "enum")
        for child in schema.get("allOf", []):
            self.validate(value, child, filename)
        for keyword, exact in (("anyOf", False), ("oneOf", True)):
            if keyword in schema:
                count = sum(self.valid(value, child, filename) for child in schema[keyword])
                require(count == 1 if exact else count >= 1, keyword)
        if "not" in schema:
            require(not self.valid(value, schema["not"], filename), "not")
        if "if" in schema:
            branch = "then" if self.valid(value, schema["if"], filename) else "else"
            self.validate(value, schema.get(branch, {}), filename)
        types = {
            "null": value is None, "boolean": type(value) is bool,
            "integer": type(value) is int,
            "number": type(value) in (int, float) and math.isfinite(value),
            "string": type(value) is str, "array": type(value) is list,
            "object": type(value) is dict,
        }
        if "type" in schema:
            # This B0 helper additionally requires integer Python values; lexical
            # integer-token enforcement in the runtime/comparator is still separate.
            require(types.get(schema["type"], False), "type")
        if type(value) in (int, float):
            require(math.isfinite(value), "finite number")
            for key, predicate in (
                ("minimum", lambda a, b: a >= b), ("maximum", lambda a, b: a <= b),
                ("exclusiveMinimum", lambda a, b: a > b),
                ("exclusiveMaximum", lambda a, b: a < b),
                ("multipleOf", lambda a, b: a % b == 0),
            ):
                if key in schema:
                    require(predicate(value, schema[key]), key)
        if isinstance(value, str):
            require(len(value) >= schema.get("minLength", 0), "minLength")
            require(len(value) <= schema.get("maxLength", math.inf), "maxLength")
            if "pattern" in schema:
                require(re.search(schema["pattern"], value) is not None, "pattern")
        if isinstance(value, list):
            require(len(value) >= schema.get("minItems", 0), "minItems")
            require(len(value) <= schema.get("maxItems", math.inf), "maxItems")
            for item in value:
                self.validate(item, schema.get("items", {}), filename)
        if isinstance(value, dict):
            require(len(value) <= schema.get("maxProperties", math.inf), "maxProperties")
            require(len(value) >= schema.get("minProperties", 0), "minProperties")
            require(all(key in value for key in schema.get("required", [])), "required")
            for key, dependencies in schema.get("dependentRequired", {}).items():
                require(key not in value or all(dep in value for dep in dependencies), "dependentRequired")
            for key, item in value.items():
                self.validate(key, schema.get("propertyNames", {}), filename)
                child = schema.get("properties", {}).get(key, schema.get("additionalProperties", {}))
                self.validate(item, child, filename)

    def sample(self, schema, filename):
        """Invented shape-only specimen. No oracle, economics or source data."""
        if "$ref" in schema:
            target, target_file = self.resolve(schema["$ref"], filename)
            return self.sample(target, target_file)
        if "const" in schema:
            return copy.deepcopy(schema["const"])
        if "enum" in schema:
            return copy.deepcopy(schema["enum"][0])
        if "anyOf" in schema:
            return self.sample(schema["anyOf"][0], filename)
        if "allOf" in schema and "type" not in schema:
            result = self.sample(schema["allOf"][0], filename)
            if isinstance(result, dict):
                for child in schema["allOf"][1:]:
                    for key in child.get("required", []):
                        if key not in result:
                            base, basefile = self.resolve(schema["allOf"][0]["$ref"], filename)
                            result[key] = self.sample(base["properties"][key], basefile)
            return result
        typ = schema.get("type")
        if typ == "object":
            properties = copy.deepcopy(schema.get("properties", {}))
            if "oneOf" in schema:
                for key, value in schema["oneOf"][0].get("properties", {}).items():
                    properties[key].update(value)
            return {key: self.sample(properties[key], filename) for key in schema.get("required", [])}
        if typ == "array":
            return [self.sample(schema["items"], filename) for _ in range(schema.get("minItems", 0))]
        if typ in ("integer", "number"):
            lower = schema.get("minimum", 0)
            return max(0, lower, schema.get("exclusiveMinimum", -1) + 1)
        if typ == "boolean":
            return False
        if typ == "null":
            return None
        if typ == "string":
            pattern = schema.get("pattern", "")
            if "{64}" in pattern:
                return "0" * 64
            if "{40}" in pattern:
                return ""
            if "197[0-9]" in pattern:
                return "1970-01-01" if "0[1-9]" in pattern else "1970"
            return "a" * max(1 if pattern else 0, schema.get("minLength", 0))
        return {}


def go_structs():
    result = {}
    sources = (
        ("report/adaptiveflagunit/types.go", ""),
        ("engine/adaptive_flag_types.go", "engine."),
        ("dsl/adaptive_flag_config.go", "dsl."),
    )
    imported = {"AdaptiveFlagEventTime", "AdaptiveFlagQueuedExit", "AdaptiveFlagState", "AdaptiveFlagSpec", "AdaptiveFlagRules"}
    for filename, prefix in sources:
        source = (ROOT / filename).read_text(encoding="utf-8")
        for name, body in re.findall(r"type\s+(\w+)\s+struct\s*\{(.*?)\n\}", source, re.S):
            if prefix and name not in imported:
                continue
            fields = {}
            for line in re.sub(r"//[^\n]*", "", body).splitlines():
                line = line.strip()
                if not line:
                    continue
                match = re.fullmatch(r'(\w+)\s+(\S+)\s+`([^`]+)`', line)
                if match:
                    _, typ, text = match.groups()
                    tags = dict(re.findall(r'(\w+):"([^"]*)"', text))
                    fields[tags["json"].split(",")[0]] = (typ, tags)
                elif re.fullmatch(r"\w+", line) and line in result:
                    fields.update(result[line])
                else:
                    raise Invalid(f"unsupported Go source shape {filename}:{name}: {line}")
            result[prefix + name] = fields
    return result


class B0SchemaTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.store = LocalSchemas()
        cls.docs = {role: cls.store.load(name) for role, name in FILES.items()}
        cls.objects = {}
        for role, doc in cls.docs.items():
            for node in [doc, *doc.get("$defs", {}).values()]:
                if "x-goType" in node:
                    cls.objects[node["x-goType"]] = (node, FILES[role])

    def check(self, value, schema, filename, passes=True):
        self.assertEqual(self.store.valid(value, schema, filename), passes)

    def test_01_exact_source_fields_presence_types_and_byte_annotations(self):
        source = go_structs()
        self.assertEqual(set(self.objects), set(source))
        for name, fields in source.items():
            node, filename = self.objects[name]
            with self.subTest(go_type=name):
                self.assertIs(node["additionalProperties"], False)
                self.assertEqual(list(node["properties"]), list(fields))
                required = [key for key, (_, tags) in fields.items() if "omitempty" not in tags["json"]]
                if name == "ClosedTrade":
                    required = list(fields)
                self.assertEqual(node["required"], required)
                for key, (typ, tags) in fields.items():
                    field = node["properties"][key]
                    nullable = typ.startswith("*") and ("omitempty" not in tags["json"] or typ.startswith("**"))
                    if nullable:
                        self.assertTrue(any(item.get("type") == "null" for item in field["anyOf"]))
                        field = field["anyOf"][0]
                    else:
                        self.assertNotIn("anyOf", field)
                    bare = typ.lstrip("*")
                    if bare in ("string", "float64", "bool", "int", "int64"):
                        self.assertEqual(field["type"], {"string": "string", "float64": "number", "bool": "boolean", "int": "integer", "int64": "integer"}[bare])
                    if bare == "float64":
                        self.assertEqual(field["x-numericType"], "binary64")
                        self.assertEqual(field["maximum"], sys.float_info.max)
                        self.assertEqual(field["minimum"], -sys.float_info.max)
                    if "unitmax" in tags:
                        self.assertEqual(field["x-maxUTF8Bytes"], int(tags["unitmax"]))
                        self.assertEqual(field["maxLength"], int(tags["unitmax"]))
                    if "unitliteral" in tags:
                        self.assertEqual(field["const"], tags["unitliteral"])
                    if bare.startswith("[]"):
                        self.assertEqual(field["type"], "array")
                    if bare.startswith("map[string]"):
                        self.assertEqual(field["type"], "object")
                    target = bare
                    if bare.startswith("[]"):
                        target, field = bare[2:], field["items"]
                    elif bare.startswith("map[string]"):
                        target, field = bare[11:], field["additionalProperties"]
                    if target not in ("string", "float64", "bool", "int", "int64", "json.RawMessage"):
                        if name.startswith("engine.") and "." not in target:
                            target = "engine." + target
                        if name.startswith("dsl.") and "." not in target:
                            target = "dsl." + target
                        resolved, _ = self.store.resolve(field["$ref"], filename)
                        if name == "Terminal" and key == "open_position":
                            resolved, _ = self.store.resolve(resolved["allOf"][0]["$ref"], filename)
                        self.assertEqual(resolved["x-goType"], target)

    def test_02_closed_objects_missing_null_unknown_mutations(self):
        for name, (node, filename) in self.objects.items():
            with self.subTest(go_type=name):
                specimen = self.store.sample(node, filename)
                self.check(specimen, node, filename)
                self.check(dict(specimen, unexpected=True), node, filename, False)
                for key in node["required"]:
                    changed = dict(specimen)
                    del changed[key]
                    self.check(changed, node, filename, False)
                self.check(None, node, filename, False)

    def test_03_optional_fill_group_and_filled_contexts(self):
        order, filename = self.objects["ProjectedOrder"]
        specimen = self.store.sample(order, filename)
        for mask in itertools.product((False, True), repeat=4):
            item = dict(specimen)
            for present, key in zip(mask, FILL):
                if present:
                    item[key] = None if key.startswith(("display_", "fill_risk_effective")) else 1.0
            self.check(item, order, filename, all(mask) or not any(mask))
        filled = dict(specimen, raw_entry=1.0, fill_risk_raw_U=1.0,
                      display_proxy_adjusted_entry=None, fill_risk_effective_U=None)
        filled_schema = self.docs["envelope"]["$defs"]["FilledProjectedOrder"]
        self.check(filled, filled_schema, filename)
        self.check(specimen, filled_schema, filename, False)
        for key in ("raw_entry", "fill_risk_raw_U"):
            self.check(dict(filled, **{key: None}), order, filename, False)
        trade, filename = self.objects["ClosedTrade"]
        specimen = self.store.sample(trade, filename)
        for key in FILL:
            item = dict(specimen)
            del item[key]
            self.check(item, trade, filename, False)

    def test_04_error_pairs_operations_locations_and_wire_annotations(self):
        node, filename = self.objects["ErrorDetail"]
        specimen = self.store.sample(node, filename)
        self.assertEqual(node["properties"]["code"]["enum"], list(PAIRS))
        for code, phase in itertools.product(PAIRS, set(PAIRS.values())):
            self.check(dict(specimen, code=code, phase=phase), node, filename, PAIRS[code] == phase)
        location, filename = self.objects["ErrorLocation"]
        self.assertEqual(location["properties"]["operation"]["anyOf"][0]["enum"], OPERATIONS)
        for operation in [None, *OPERATIONS]:
            item = dict(operation=operation, rowIndex=None, orderId=None, eventId=None)
            self.check(item, location, filename)
        self.check(dict(item, operation="invented"), location, filename, False)
        for key, maximum in (("rowIndex", 1023), ("orderId", 1023), ("eventId", 4095)):
            for value in (-1, maximum, maximum + 1, None, False, 1.5):
                self.check(dict(item, **{key: value}), location, filename, value is None or type(value) is int and value == maximum)
        self.assertEqual(self.docs["error"]["x-maxWireBytesIncludingNewline"], 8192)
        message = node["properties"]["message"]
        self.check("x" * 1024, message, FILES["error"])
        self.check("x" * 1025, message, FILES["error"], False)
        # Deliberately demonstrates why the UTF-8 byte obligation is annotated.
        self.check("é" * 1024, message, FILES["error"])
        self.assertGreater(len(("é" * 1024).encode("utf-8")), message["x-maxUTF8Bytes"])

    def test_05_cardinalities_empty_collections_and_year_keys(self):
        node, filename = self.objects["Projection"]
        limits = {"orders": N, "invalid_planned_risk_rejections": N, "cost_events": 2*N,
                  "closed_trades": N, "marks": N, "daily": D}
        for key, maximum in limits.items():
            field = node["properties"][key]
            self.assertEqual(field["maxItems"], maximum)
            sample = self.store.sample(field["items"], filename)
            self.check([sample] * maximum, field, filename)
            self.check([sample] * (maximum + 1), field, filename, False)
            self.check(None, field, filename, False)
            self.check([], field, filename, key != "daily")
        exposure, filename = self.objects["Exposure"]
        for key, maximum in (("quote_gaps_bridged", G), ("utc_midnights_definitely_crossed", E), ("utc_midnights_possibly_crossed", E)):
            field = exposure["properties"][key]
            self.assertEqual(field["maxItems"], maximum)
            value = self.store.sample(field["items"], filename)
            self.check([value] * maximum, field, filename)
            self.check([value] * (maximum + 1), field, filename, False)
        for key in ("by_entry_year_cohort", "by_calendar_year_marked_change"):
            field = node["properties"][key]
            self.assertEqual(field["maxProperties"], D)
            value = self.store.sample(field["additionalProperties"], filename)
            self.check({str(1970+i): value for i in range(D)}, field, filename)
            self.check({str(1970+i): value for i in range(D+1)}, field, filename, False)
            for year in ("1969", "10000", "02000", "year"):
                self.check({year: value}, field, filename, False)
            self.check({}, field, filename)
            self.check(None, field, filename, False)
        self.assertEqual({key: self.docs["envelope"]["x-resourceLimits"][key] for key in "NDEG"}, dict(N=N, D=D, E=E, G=G))

    def test_06_finite_numbers_scalar_types_and_no_normalization(self):
        order, filename = self.objects["ProjectedOrder"]
        floating = order["properties"]["raw_trigger"]
        for value in (0, -0.0, 1.0, sys.float_info.max, -sys.float_info.max, float.fromhex("0x0.0000000000001p-1022")):
            self.check(value, floating, filename)
        for value in (True, False, None, "1", math.inf, -math.inf, math.nan):
            self.check(value, floating, filename, False)
        count = order["properties"]["order_id"]
        for value in (False, 1.5, "1", None, -1, 1024):
            self.check(value, count, filename, False)
        self.check(1023, count, filename)

    def test_07_request_identity_policy_strings_and_fixed_profiles(self):
        request = self.docs["request"]
        filename = FILES["request"]
        sample = self.store.sample(request, filename)
        self.check(sample, request, filename)
        for key, value in (("costPolicy", "CUSTOM"), ("scenario", "ACCOUNT"), ("numericalPolicy", "DECIMAL")):
            self.check(dict(sample, **{key: value}), request, filename, False)
        data = request["$defs"]["DataSource"]
        for ident, passes in (("a", True), ("a"*128, True), ("a"*129, False), ("", False), ("a b", False), ("é", False), ("a\n", False), ("dataset:revision/a-b_1.2", True)):
            self.check(dict(id=ident, sourceSha256="0"*64), data, filename, passes)
        policy, filename = self.objects["CostPolicy"]
        rows = [("RAW", 0, 0, 0, None), ("RAZOR_PROXY_BASIC", .095, .06, .035, None),
                ("RAZOR_PROXY_HARSH_AGGREGATE", .155, None, None, .155)]
        for row in rows:
            value = dict(zip(("name", "per_fill", "spread", "commission", "aggregate"), row))
            self.check(value, policy, filename)
            self.check(dict(value, per_fill=999), policy, filename, False)
        numerical, _ = self.objects["NumericalPolicy"]
        for key, value in (("sums", "chronological positive-zero"), ("operations", "separate binary64"), ("comparisons", "direct")):
            self.assertEqual(numerical["properties"][key]["const"], value)
        build, filename = self.objects["BuildIdentity"]
        specimen = self.store.sample(build, filename)
        for value, valid in (("", True), ("a"*40, True), (None, False), ("A"*40, False), ("a"*41, False), ("\n", False)):
            self.check(dict(specimen, vcsRevision=value), build, filename, valid)
        self.check(dict(specimen, vcsModified=None), build, filename)

    def test_08_raw_reference_and_existing_schemas_unchanged(self):
        self.assertEqual(self.docs["envelope"]["properties"]["raw"], {"$ref": "adaptive-flag-runtime-envelope-v1.schema.json"})
        for filename, digest in STAGE_A_SHA256.items():
            self.assertEqual(hashlib.sha256((SCHEMAS / filename).read_bytes()).hexdigest(), digest)
        for doc in self.docs.values():
            self.assertTrue(doc["x-runtimeConstraints"])
            self.assertEqual(doc["$schema"], "https://json-schema.org/draft/2020-12/schema")

    def test_09_all_schema_refs_and_test_validator_keyword_coverage(self):
        supported = {
            "$schema", "$id", "$ref", "$defs", "title", "description", "$comment",
            "type", "properties", "required", "additionalProperties", "propertyNames",
            "minProperties", "maxProperties", "items", "minItems", "maxItems",
            "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "multipleOf",
            "minLength", "maxLength", "pattern", "const", "enum", "anyOf", "oneOf",
            "allOf", "not", "if", "then", "else", "dependentRequired",
        }
        visited = set()

        def walk(node, filename):
            if type(node) is bool:
                return
            self.assertTrue(set(node).issubset(supported | {key for key in node if key.startswith("x-")}))
            if "$ref" in node:
                key = (filename, node["$ref"])
                if key not in visited:
                    visited.add(key)
                    target, target_file = self.store.resolve(node["$ref"], filename)
                    walk(target, target_file)
            for key in ("$defs", "properties"):
                for child in node.get(key, {}).values():
                    walk(child, filename)
            for key in ("anyOf", "oneOf", "allOf"):
                for child in node.get(key, []):
                    walk(child, filename)
            for key in ("additionalProperties", "propertyNames", "items", "not", "if", "then", "else"):
                if key in node:
                    walk(node[key], filename)

        for role, node in self.docs.items():
            walk(node, FILES[role])
        exposure, filename = self.objects["Exposure"]
        annotations = " ".join(exposure["x-runtimeConstraints"])
        self.assertIn("E<=732", annotations)
        self.assertIn("G<=1024", annotations)
        self.assertEqual(self.docs["request"]["x-maxInputUTF8Bytes"], 1024)
        # Both objects independently satisfy their local maxima. A standard
        # schema cannot establish the aggregate resource budget across them.
        item = self.store.sample(exposure, filename)
        item["utc_midnights_definitely_crossed"] = [86400000] * E
        self.check(item, exposure, filename)
        other = copy.deepcopy(item)
        other["utc_midnights_definitely_crossed"] = [86400000]
        self.check(other, exposure, filename)
        self.assertEqual(len(item["utc_midnights_definitely_crossed"]) + len(other["utc_midnights_definitely_crossed"]), E + 1)


if __name__ == "__main__":
    unittest.main(verbosity=2)
