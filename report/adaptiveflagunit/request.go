package adaptiveflagunit

import (
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

// DecodeRuntimeRequest admits the closed projection metadata object. The byte
// bound is checked before decoding, and token decoding preserves duplicate keys
// after JSON unescaping. Every accepted key and value is closed ASCII, so an
// escaped surrogate or replacement rune cannot become an admitted string.
func DecodeRuntimeRequest(raw string) (Request, error) {
	if len(raw) > MaxProjectionMetadataBytes {
		return Request{}, newCoreError("resource_limit", "projection_request", -1, -1, -1, "projection metadata exceeds 1024 bytes")
	}
	if !utf8.ValidString(raw) {
		return Request{}, projectionRequestError("projection metadata requires valid UTF-8")
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return Request{}, projectionRequestError("projection metadata requires one object")
	}
	var out Request
	seen := map[string]bool{}
	for d.More() {
		keyToken, err := d.Token()
		key, ok := keyToken.(string)
		if err != nil || !ok || seen[key] {
			return Request{}, projectionRequestError("projection metadata has a duplicate or invalid key")
		}
		seen[key] = true
		switch key {
		case "schema", "scenario", "numericalPolicy", "costPolicy":
			value, err := d.Token()
			text, ok := value.(string)
			if err != nil || !ok {
				return Request{}, projectionRequestError("projection metadata requires exact string values")
			}
			switch key {
			case "schema":
				out.Schema = text
			case "scenario":
				out.Scenario = text
			case "numericalPolicy":
				out.NumericalPolicy = text
			case "costPolicy":
				out.CostPolicy = text
			}
		case "dataSource":
			out.DataSource, err = decodeRuntimeDataSource(d)
			if err != nil {
				return Request{}, err
			}
		default:
			return Request{}, projectionRequestError("projection metadata has an unknown field")
		}
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') {
		return Request{}, projectionRequestError("projection metadata has a malformed object")
	}
	if _, err := d.Token(); err != io.EOF {
		return Request{}, projectionRequestError("projection metadata has trailing input")
	}
	for _, key := range []string{"schema", "scenario", "numericalPolicy", "costPolicy", "dataSource"} {
		if !seen[key] {
			return Request{}, projectionRequestError("projection metadata is missing a mandatory field")
		}
	}
	if out.Schema != RequestSchema || out.Scenario != "UNIT_POINT_VALUE_1" || out.NumericalPolicy != "BINARY64_ORDERED_V1" {
		return Request{}, projectionRequestError("projection metadata has an unsupported schema, scenario or numerical policy")
	}
	if _, err := policyFor(out.CostPolicy); err != nil {
		return Request{}, err
	}
	return out, nil
}

func decodeRuntimeDataSource(d *json.Decoder) (DataSource, error) {
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return DataSource{}, projectionRequestError("projection data source requires an object")
	}
	var out DataSource
	seen := map[string]bool{}
	for d.More() {
		keyToken, err := d.Token()
		key, ok := keyToken.(string)
		if err != nil || !ok || seen[key] || (key != "id" && key != "sourceSha256") {
			return DataSource{}, projectionRequestError("projection data source has a duplicate, unknown or invalid key")
		}
		seen[key] = true
		value, err := d.Token()
		text, ok := value.(string)
		if err != nil || !ok {
			return DataSource{}, projectionRequestError("projection data source requires string values")
		}
		if key == "id" {
			out.ID = text
		} else {
			out.SourceSHA256 = text
		}
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') || !seen["id"] || !seen["sourceSha256"] {
		return DataSource{}, projectionRequestError("projection data source is malformed or incomplete")
	}
	if !validIdentitySourceID(out.ID) || !lowercaseHexOfLength(out.SourceSHA256, 64) {
		return DataSource{}, projectionRequestError("projection data source identity or digest is invalid")
	}
	return out, nil
}

func projectionRequestError(message string) error {
	return newCoreError("projection_request_rejected", "projection_request", -1, -1, -1, message)
}
