"""Generic, field-typed lossless JSON comparison. No accounting implementation.

Numeric JSON lexemes survive until the declarative schema selects integer or
binary64 comparison. In particular, an integer-looking -0 is never decoded by
Python's int before a float64 field sees it. No retained oracle is imported.
"""
from __future__ import annotations

import argparse
from dataclasses import dataclass
import json
import math
from pathlib import Path
import re
import struct
from typing import Any


@dataclass(frozen=True)
class NumberToken:
    text: str


class InvalidJSON(ValueError):
    pass


INTEGER = re.compile(r"-?(?:0|[1-9][0-9]*)\Z")
NUMBER = re.compile(r"-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?\Z")
MISSING = object()


def loads_lossless(text: str | bytes) -> Any:
    """Strict JSON, rejecting duplicates, nonfinite constants and surrogates."""
    if isinstance(text, bytes):
        text = text.decode("utf-8", errors="strict")
    if not isinstance(text, str):
        raise TypeError("JSON input must be str or UTF-8 bytes")

    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise InvalidJSON(f"duplicate decoded key: {key!r}")
            result[key] = value
        return result

    def constant(token):
        raise InvalidJSON(f"non-JSON number: {token}")

    try:
        value = json.loads(text, parse_int=NumberToken, parse_float=NumberToken,
                           parse_constant=constant, object_pairs_hook=pairs)
    except json.JSONDecodeError as exc:
        raise InvalidJSON(str(exc)) from exc

    def unicode_check(node):
        if isinstance(node, str):
            if any(0xD800 <= ord(c) <= 0xDFFF for c in node):
                raise InvalidJSON("unpaired Unicode surrogate")
        elif isinstance(node, dict):
            for key, child in node.items():
                unicode_check(key)
                unicode_check(child)
        elif isinstance(node, list):
            for child in node:
                unicode_check(child)
    unicode_check(value)
    return value


def float_bits(number: float) -> str:
    return "0x" + struct.pack(">d", number).hex()


def describe(value: Any) -> dict:
    if value is MISSING:
        return {"type": "missing"}
    if isinstance(value, NumberToken):
        out = {"type": "number-token", "token": value.text}
        try:
            number = float(value.text)
            if math.isfinite(number):
                out["binary64_bits"] = float_bits(number)
        except (OverflowError, ValueError):
            pass
        return out
    if type(value) is float:
        return {"type": "python-float", "repr": repr(value),
                "binary64_bits": float_bits(value)}
    if value is None:
        return {"type": "null"}
    if isinstance(value, (dict, list)):
        return {"type": "object" if isinstance(value, dict) else "array",
                "size": len(value)}
    return {"type": type(value).__name__, "value": value}


def _pointer(path, key):
    return path + "/" + str(key).replace("~", "~0").replace("/", "~1")


def resolve(schema: dict, definitions: dict) -> dict:
    while "$ref" in schema:
        ref = schema["$ref"]
        if not ref.startswith("#/$defs/"):
            raise ValueError(f"unsupported schema reference: {ref}")
        overrides = {k: v for k, v in schema.items() if k != "$ref"}
        schema = {**definitions[ref[len("#/$defs/"):]], **overrides}
    return schema


def compare_values(expected: Any, actual: Any, schema: dict, *,
                   definitions: dict | None = None) -> list[dict]:
    """Return mismatches; [] means exact equality under a CLOSED typed schema.

    JSON inputs must come from loads_lossless. An independent Python expected
    tree is also accepted: float64 leaves must be Python float, integer leaves
    Python int. Actual numeric leaves must ALWAYS be preserved NumberTokens.
    """
    definitions = definitions or {}
    errors = []

    def error(path, reason, left, right):
        errors.append({"path": path or "/", "reason": reason,
                       "expected": describe(left), "actual": describe(right)})

    def scalar(value, spec, side, path, left, right):
        kind = spec["type"]
        if value is None:
            if spec.get("nullable", False) or kind == "null":
                return ("null", None)
            error(path, f"{side}: null forbidden", left, right)
            return None
        result = None
        if kind == "float64":
            if isinstance(value, NumberToken):
                if NUMBER.fullmatch(value.text) is None:
                    error(path, f"{side}: invalid JSON numeric token", left, right)
                    return None
                try:
                    number = float(value.text)
                except (OverflowError, ValueError):
                    number = float("inf")
            elif side == "expected" and type(value) is float:
                number = value
            else:
                error(path, f"{side}: expected float64 numeric token", left, right)
                return None
            if not math.isfinite(number):
                error(path, f"{side}: nonfinite binary64", left, right)
                return None
            result = ("float64", float_bits(number))
        elif kind == "integer":
            if isinstance(value, NumberToken) and INTEGER.fullmatch(value.text):
                try:
                    number = int(value.text)
                except ValueError:
                    error(path, f"{side}: integer token exceeds decoder resource limit", left, right)
                    return None
            elif side == "expected" and type(value) is int:
                number = value
            else:
                error(path, f"{side}: expected integer token without fraction or exponent", left, right)
                return None
            if ("minimum" in spec and number < spec["minimum"]) or ("maximum" in spec and number > spec["maximum"]):
                error(path, f"{side}: integer outside declared range", left, right)
                return None
            result = ("integer", number)
        elif kind == "boolean" and type(value) is bool:
            result = (kind, value)
        elif kind == "string" and type(value) is str:
            if ("pattern" in spec and re.fullmatch(spec["pattern"], value) is None) or ("maxLength" in spec and len(value) > spec["maxLength"]):
                error(path, f"{side}: string outside declared domain", left, right)
                return None
            result = (kind, value)
        elif kind == "object" and type(value) is dict:
            if "maxProperties" in spec and len(value) > spec["maxProperties"]:
                error(path, f"{side}: object outside declared cardinality", left, right)
                return None
            result = (kind, None)
        elif kind == "array" and type(value) is list:
            if ("minItems" in spec and len(value) < spec["minItems"]) or ("maxItems" in spec and len(value) > spec["maxItems"]):
                error(path, f"{side}: array outside declared cardinality", left, right)
                return None
            result = (kind, None)
        if result is None:
            error(path, f"{side}: expected {kind}", left, right)
        elif "const" in spec and value != spec["const"]:
            error(path, f"{side}: incorrect declared constant", left, right)
            return None
        elif "enum" in spec and value not in spec["enum"]:
            error(path, f"{side}: outside declared enum", left, right)
            return None
        return result

    def visit(left, right, spec, path):
        spec = resolve(spec, definitions)
        if left is MISSING or right is MISSING:
            if left is not right:
                error(path, "field presence differs", left, right)
            return
        a = scalar(left, spec, "expected", path, left, right)
        b = scalar(right, spec, "actual", path, left, right)
        if a is None or b is None:
            return
        if a != b:
            error(path, "typed values differ", left, right)
            return
        if a[0] == "null":
            return
        kind = spec["type"]
        if kind == "object":
            fields = spec.get("properties", {})
            dynamic = spec.get("values")
            if dynamic is not None and fields:
                raise ValueError("schema cannot mix named fields and map values")
            for side, obj in (("expected", left), ("actual", right)):
                for key in spec.get("required", fields if dynamic is None else []):
                    if key not in obj:
                        error(_pointer(path, key), f"{side}: missing required field", left.get(key, MISSING), right.get(key, MISSING))
                for group in spec.get("presenceGroups", []):
                    if any(key in obj for key in group) and not all(key in obj for key in group):
                        error(path, f"{side}: partial field presence group {group!r}", left, right)
            for key in sorted(left.keys() | right.keys()):
                child_path = _pointer(path, key)
                if dynamic is None and key not in fields:
                    error(child_path, "unknown field", left.get(key, MISSING), right.get(key, MISSING))
                    continue
                if dynamic is not None and re.fullmatch(spec["keyPattern"], key) is None:
                    error(child_path, "map key outside declared domain", left.get(key, MISSING), right.get(key, MISSING))
                    continue
                visit(left.get(key, MISSING), right.get(key, MISSING), dynamic or fields[key], child_path)
        elif kind == "array":
            if len(left) != len(right):
                error(path, "array length differs", left, right)
            for index in range(min(len(left), len(right))):
                visit(left[index], right[index], spec["items"], _pointer(path, index))

    visit(expected, actual, schema, "")
    return errors


def compare_json(expected: str | bytes, actual: str | bytes, schema: dict, *,
                 definitions: dict | None = None) -> list[dict]:
    return compare_values(loads_lossless(expected), loads_lossless(actual), schema,
                          definitions=definitions)


def _expand(value: Any, template: str) -> dict[tuple, tuple[str, Any]]:
    """Expand JSON pointers with explicit array/map member '*' placeholders."""
    parts = [] if template == "" else template.lstrip("/").split("/")
    result = {}

    def walk(node, offset, bindings, path):
        if offset == len(parts):
            result[bindings] = (path, node)
            return
        part = parts[offset].replace("~1", "/").replace("~0", "~")
        if part == "*":
            if type(node) is list:
                entries = enumerate(node)
            elif type(node) is dict:
                entries = sorted(node.items())
            else:
                return  # Parent container/null/presence is separately mapped.
            for key, child in entries:
                walk(child, offset + 1, (*bindings, key), _pointer(path, key))
        else:
            if type(node) is not dict:
                return
            walk(node.get(part, MISSING), offset + 1, bindings, _pointer(path, part))
    walk(value, 0, (), "")
    return result


def schema_paths(schema: dict, definitions: dict, path: str = "") -> dict[str, dict]:
    """Enumerate every declared branch and leaf, with explicit member paths."""
    spec = resolve(schema, definitions)
    result = {path: spec}
    if spec["type"] == "object":
        if "values" in spec:
            result.update(schema_paths(spec["values"], definitions, path + "/*"))
        else:
            for name, child in spec.get("properties", {}).items():
                result.update(schema_paths(child, definitions, _pointer(path, name)))
    elif spec["type"] == "array":
        result.update(schema_paths(spec["items"], definitions, path + "/*"))
    return result


def validate_mapping(mapping: dict) -> None:
    """Fail closed on unmapped/duplicate paths, type drift or subtree exclusions."""
    defs = mapping["$defs"]
    expected = {side: schema_paths(mapping[root], defs) for side, root in
                (("reference", "referenceRoot"), ("go", "goRoot"))}
    seen = {"reference": set(), "go": set()}
    for rule in mapping["fields"]:
        mode = rule["mode"]
        if mode not in ("equal", "structure", "schema_only", "evidence_only"):
            raise ValueError(f"unknown mapping mode: {mode}")
        for side in seen:
            path = rule.get(side)
            if path is None:
                continue
            if path in seen[side] or path not in expected[side]:
                raise ValueError(f"duplicate or undeclared {side} mapping path: {path}")
            seen[side].add(path)
            spec = expected[side][path]
            if rule["type"] != spec["type"] or rule.get("nullable", False) != spec.get("nullable", False):
                raise ValueError(f"mapped type/nullability differs: {side} {path}")
            if mode == "schema_only" and (spec["type"] != "object" or spec.get("nullable") or "values" in spec):
                raise ValueError("schema_only is limited to closed required nonnullable objects with individually mapped descendants")
        if mode == "evidence_only" and rule["type"] in ("object", "array"):
            raise ValueError("provenance exclusions must identify individual leaves")
        if mode == "equal" and rule["type"] in ("object", "array"):
            raise ValueError("containers require explicit structure mapping")
        if mode in ("equal", "structure") and (rule.get("reference") is None or rule.get("go") is None):
            raise ValueError("comparison requires both declared paths")
    for side in seen:
        missing = expected[side].keys() - seen[side]
        if missing:
            raise ValueError(f"unmapped {side} paths: {sorted(missing)}")


def compare_mapped_json(expected: str | bytes, actual: str | bytes, mapping: dict) -> dict:
    """Compare an explicit per-field mapping, retaining noncomparable evidence.

    Neither a subtree exclusion nor arithmetic transformations are supported.
    Each version is closed-schema validated first. Renames are declarative paths;
    excluded provenance leaves must be individually listed as evidence_only.
    The caller must still independently authenticate provenance evidence.
    """
    validate_mapping(mapping)
    left, right = loads_lossless(expected), loads_lossless(actual)
    defs = mapping["$defs"]
    errors = []
    evidence = []
    for side, value, schema_key in (("reference", left, "referenceRoot"),
                                    ("go", right, "goRoot")):
        for issue in compare_values(value, value, mapping[schema_key], definitions=defs):
            errors.append({"side": side, **issue})
    if errors:
        return {"pass": False, "mismatches": errors, "evidence": evidence}
    for rule in mapping["fields"]:
        a = _expand(left, rule["reference"]) if rule.get("reference") is not None else {}
        b = _expand(right, rule["go"]) if rule.get("go") is not None else {}
        mode = rule["mode"]
        if mode == "schema_only":
            # Only closed nonnullable containers are allowed here. Their shape
            # was checked above; every descendant has its own exhaustive rule.
            continue
        if mode == "evidence_only":
            if rule["type"] in ("object", "array"):
                raise ValueError("provenance exclusions must identify individual leaves")
            evidence.append({"reference_path": rule.get("reference"), "go_path": rule.get("go"),
                             "reason": rule["reason"],
                             "reference": [{"path": path, **describe(value)} for path, value in a.values()],
                             "go": [{"path": path, **describe(value)} for path, value in b.values()]})
            continue
        if mode not in ("equal", "structure"):
            raise ValueError(f"unsupported comparison mode {mode!r}")
        for binding in sorted(a.keys() | b.keys(), key=repr):
            ap, av = a.get(binding, (rule.get("reference"), MISSING))
            bp, bv = b.get(binding, (rule.get("go"), MISSING))
            reason = None
            if av is MISSING or bv is MISSING:
                if av is not bv:
                    reason = "field/member presence differs"
            elif mode == "structure":
                if (av is None) != (bv is None):
                    reason = "container null status differs"
                elif av is not None and (type(av) is not type(bv)):
                    reason = "container type differs"
                elif type(av) is list and len(av) != len(bv):
                    reason = "array length differs"
                elif rule.get("mapKeys") and type(av) is dict and av.keys() != bv.keys():
                    reason = "map keys differ"
            else:
                spec = {"type": rule["type"], "nullable": rule.get("nullable", False)}
                for issue in compare_values(av, bv, spec):
                    errors.append({**issue, "reference_path": ap, "go_path": bp})
            if reason:
                errors.append({"reference_path": ap, "go_path": bp, "reason": reason,
                               "expected": describe(av), "actual": describe(bv)})
    return {"pass": not errors, "mismatches": errors, "evidence": evidence,
            "provenance_verification": "required separately; this receipt is not artifact authentication"}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument("--schema", type=Path, help="declarative schema with root and optional $defs")
    group.add_argument("--mapping", type=Path, help="complete closed-schema per-leaf reference/Go mapping")
    parser.add_argument("--expected", type=Path, required=True)
    parser.add_argument("--actual", type=Path, required=True)
    args = parser.parse_args()
    try:
        declaration = json.loads((args.schema or args.mapping).read_text(encoding="utf-8"))
        if args.mapping:
            receipt = compare_mapped_json(args.expected.read_bytes(), args.actual.read_bytes(), declaration)
        else:
            errors = compare_json(args.expected.read_bytes(), args.actual.read_bytes(),
                                  declaration["root"], definitions=declaration.get("$defs", {}))
            receipt = {"pass": not errors, "mismatches": errors}
    except (InvalidJSON, UnicodeError) as exc:
        receipt = {"pass": False, "mismatches": [{"reason": "invalid JSON", "detail": str(exc)}]}
    print(json.dumps({"schema": "lossless-json-comparison-receipt-v1",
                      **receipt}, allow_nan=False))
    return int(not receipt["pass"])


if __name__ == "__main__":
    raise SystemExit(main())
