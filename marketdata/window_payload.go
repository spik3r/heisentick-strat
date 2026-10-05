package marketdata

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
)

const WindowPayloadRequestSchema = "market-data-window-request-v1"
const WindowPayloadResultSchema = "market-data-window-result-v1"
const MaxWindowPayloadBytes = 32 << 20

type WindowPayloadRequest struct {
	Schema string `json:"schema"`
	Format string `json:"format"`
	FromMS *int64 `json:"fromMs"`
	ToMS   *int64 `json:"toMs"`
}
type WindowPayloadResult struct {
	Schema        string      `json:"schema"`
	Format        string      `json:"format"`
	Normalization string      `json:"normalization"`
	RequestSHA256 string      `json:"requestSha256"`
	PayloadSHA256 string      `json:"payloadSha256"`
	RowsSHA256    string      `json:"rowsSha256"`
	FromMS        *int64      `json:"fromMs"`
	ToMS          *int64      `json:"toMs"`
	SourceBars    int         `json:"sourceBars"`
	Bars          [][]float64 `json:"bars"`
}

func windowDigest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// WindowPayload validates the ORIGINAL complete payload before selecting a
// window. No row repair, missing-volume default, extra-column discard, invalid
// binary fallback, sorting or duplicate removal is allowed. JSON envelope
// metadata is opaque and remains bound by the original byte digest.
func WindowPayload(request, payload []byte) (WindowPayloadResult, error) {
	var out WindowPayloadResult
	if len(request) > 4096 || len(payload) == 0 || len(payload) > MaxWindowPayloadBytes {
		return out, fmt.Errorf("window payload/request exceeds bounds")
	}
	fields, err := windowObject(request)
	if err != nil {
		return out, err
	}
	if len(fields) != 4 {
		return out, fmt.Errorf("window requires exactly schema, format, fromMs and toMs")
	}
	for key := range fields {
		if key != "schema" && key != "format" && key != "fromMs" && key != "toMs" {
			return out, fmt.Errorf("unknown window field %s", key)
		}
	}
	var options WindowPayloadRequest
	if err := json.Unmarshal(request, &options); err != nil {
		return out, err
	}
	if options.Schema != WindowPayloadRequestSchema || (options.Format != "json" && options.Format != "btb1") {
		return out, fmt.Errorf("unsupported window schema/format")
	}
	for _, value := range []*int64{options.FromMS, options.ToMS} {
		if value != nil && (*value < 0 || *value > 9007199254740991) {
			return out, fmt.Errorf("invalid window bound")
		}
	}
	if options.FromMS != nil && options.ToMS != nil && *options.FromMS > *options.ToMS {
		return out, fmt.Errorf("reversed window bounds")
	}
	var rows [][]float64
	if options.Format == "json" {
		rows, err = windowJSONRows(payload)
	} else {
		rows, err = windowBinaryRows(payload)
	}
	if err != nil {
		return out, err
	}
	selected := make([][]float64, 0)
	for i, row := range rows {
		if len(row) != 6 {
			return out, fmt.Errorf("bars[%d] requires exactly six OHLCV values", i)
		}
		for _, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return out, fmt.Errorf("bars[%d] must contain finite numbers", i)
			}
		}
		if row[0] < 0 || row[0] > 9007199254740991 || math.Trunc(row[0]) != row[0] || (i > 0 && row[0] <= rows[i-1][0]) || row[5] < 0 || row[2] < math.Max(row[1], row[4]) || row[3] > math.Min(row[1], row[4]) || row[2] < row[3] {
			return out, fmt.Errorf("bars[%d] has invalid ordered timestamps or OHLCV bounds", i)
		}
		if (options.FromMS == nil || row[0] >= float64(*options.FromMS)) && (options.ToMS == nil || row[0] <= float64(*options.ToMS)) {
			selected = append(selected, row)
		}
	}
	if len(selected) > 30000 {
		return out, fmt.Errorf("window exceeds 30000 selected bars")
	}
	encoded, err := json.Marshal(selected)
	if err != nil {
		return out, err
	}
	return WindowPayloadResult{Schema: WindowPayloadResultSchema, Format: options.Format,
		Normalization: "strict-six-number-ohlcv-window-v1", RequestSHA256: windowDigest(request), PayloadSHA256: windowDigest(payload), RowsSHA256: windowDigest(encoded), FromMS: options.FromMS, ToMS: options.ToMS, SourceBars: len(rows), Bars: selected}, nil
}

func windowObject(raw []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return nil, fmt.Errorf("window JSON requires an object")
	}
	out := map[string]json.RawMessage{}
	for d.More() {
		keyToken, err := d.Token()
		if err != nil {
			return nil, err
		}
		key := keyToken.(string)
		if _, duplicate := out[key]; duplicate {
			return nil, fmt.Errorf("duplicate window JSON field %s", key)
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing window JSON")
	}
	return out, nil
}

func windowJSONRows(payload []byte) ([][]float64, error) {
	fields, err := windowObject(payload)
	if err != nil {
		return nil, err
	}
	raw, exists := fields["bars"]
	if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("window JSON requires a bars array")
	}
	var records []json.RawMessage
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	rows := make([][]float64, len(records))
	for i, record := range records {
		var values []json.RawMessage
		if json.Unmarshal(record, &values) != nil || len(values) != 6 {
			return nil, fmt.Errorf("bars[%d] requires exactly six OHLCV values", i)
		}
		rows[i] = make([]float64, 6)
		for j, value := range values {
			d := json.NewDecoder(bytes.NewReader(value))
			d.UseNumber()
			var decoded any
			if d.Decode(&decoded) != nil {
				return nil, fmt.Errorf("bars[%d][%d] is not numeric", i, j)
			}
			number, ok := decoded.(json.Number)
			if !ok {
				return nil, fmt.Errorf("bars[%d][%d] is not numeric", i, j)
			}
			rows[i][j], err = number.Float64()
			if err != nil {
				return nil, fmt.Errorf("bars[%d][%d] is not finite", i, j)
			}
		}
	}
	return rows, nil
}

func windowBinaryRows(payload []byte) ([][]float64, error) {
	if len(payload) < 16 || binary.LittleEndian.Uint32(payload[:4]) != btb1Magic || binary.LittleEndian.Uint32(payload[4:8]) != 1 {
		return nil, fmt.Errorf("invalid BTB1 header")
	}
	n, columns := uint64(binary.LittleEndian.Uint32(payload[8:12])), binary.LittleEndian.Uint32(payload[12:16])
	if columns != 6 || uint64(len(payload)) != 16+n*6*8 {
		return nil, fmt.Errorf("BTB1 requires exactly six columns and exact byte length")
	}
	rows := make([][]float64, int(n))
	for i := range rows {
		rows[i] = make([]float64, 6)
		for j := range rows[i] {
			offset := 16 + (j*int(n)+i)*8
			rows[i][j] = math.Float64frombits(binary.LittleEndian.Uint64(payload[offset : offset+8]))
		}
	}
	return rows, nil
}
