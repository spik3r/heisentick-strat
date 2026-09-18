package marketdata

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const (
	btb1Magic       uint32 = 0x31425442
	btb1Version     uint32 = 1
	btb1HeaderBytes        = 16
	btb1F64Bytes           = 8
)

// DecodeBTB1 decodes the repository's BTB1 bar binary format, produced by
// engine/columnarBars.js. Columns are little-endian Float64 arrays ordered as
// t,o,h,l,c,v. Extra columns are ignored.
func DecodeBTB1(data []byte) (Series, error) {
	if len(data) < btb1HeaderBytes || binary.LittleEndian.Uint32(data[0:4]) != btb1Magic {
		return Series{}, fmt.Errorf("invalid bar binary magic")
	}
	version := binary.LittleEndian.Uint32(data[4:8])
	if version != btb1Version {
		return Series{}, fmt.Errorf("unsupported bar binary version %d", version)
	}
	n := int(binary.LittleEndian.Uint32(data[8:12]))
	colCount := int(binary.LittleEndian.Uint32(data[12:16]))
	if colCount < 5 {
		return Series{}, fmt.Errorf("invalid bar binary column count %d", colCount)
	}
	need := btb1HeaderBytes + n*colCount*btb1F64Bytes
	if n < 0 || need < btb1HeaderBytes || len(data) < need {
		return Series{}, fmt.Errorf("invalid bar binary length")
	}

	series := NewSeries(n)
	columns := [][]float64{series.T, series.O, series.H, series.L, series.C, series.V}
	copyCount := min(colCount, len(columns))
	for col := 0; col < copyCount; col++ {
		offset := btb1HeaderBytes + col*n*btb1F64Bytes
		for i := 0; i < n; i++ {
			start := offset + i*btb1F64Bytes
			columns[col][i] = math.Float64frombits(binary.LittleEndian.Uint64(data[start : start+btb1F64Bytes]))
		}
	}
	return series, nil
}

// DecodeBTB1Reader decodes BTB1 data from r without retaining the whole file.
func DecodeBTB1Reader(r io.Reader) (Series, error) {
	header := make([]byte, btb1HeaderBytes)
	if n, err := io.ReadFull(r, header); err != nil {
		if n >= 4 && binary.LittleEndian.Uint32(header[0:4]) != btb1Magic {
			return Series{}, fmt.Errorf("invalid bar binary magic")
		}
		return Series{}, fmt.Errorf("read bar binary header: %w", err)
	}
	if binary.LittleEndian.Uint32(header[0:4]) != btb1Magic {
		return Series{}, fmt.Errorf("invalid bar binary magic")
	}
	version := binary.LittleEndian.Uint32(header[4:8])
	if version != btb1Version {
		return Series{}, fmt.Errorf("unsupported bar binary version %d", version)
	}
	n := int(binary.LittleEndian.Uint32(header[8:12]))
	colCount := int(binary.LittleEndian.Uint32(header[12:16]))
	if n < 0 || colCount < 5 {
		return Series{}, fmt.Errorf("invalid bar binary shape")
	}

	series := NewSeries(n)
	columns := [][]float64{series.T, series.O, series.H, series.L, series.C, series.V}
	buf := make([]byte, btb1F64Bytes)
	for col := 0; col < colCount; col++ {
		var values []float64
		if col < len(columns) {
			values = columns[col]
		}
		for i := 0; i < n; i++ {
			if _, err := io.ReadFull(r, buf); err != nil {
				return Series{}, fmt.Errorf("read bar binary column %d row %d: %w", col, i, err)
			}
			if values != nil {
				values[i] = math.Float64frombits(binary.LittleEndian.Uint64(buf))
			}
		}
	}
	return series, nil
}

// EncodeBTB1 encodes a Series into the JS-compatible BTB1 format. It is used
// by tests and future fixture generators.
func EncodeBTB1(series Series) []byte {
	n := series.Len()
	colCount := 6
	data := make([]byte, btb1HeaderBytes+n*colCount*btb1F64Bytes)
	binary.LittleEndian.PutUint32(data[0:4], btb1Magic)
	binary.LittleEndian.PutUint32(data[4:8], btb1Version)
	binary.LittleEndian.PutUint32(data[8:12], uint32(n))
	binary.LittleEndian.PutUint32(data[12:16], uint32(colCount))

	columns := [][]float64{series.T, series.O, series.H, series.L, series.C, series.V}
	for col, values := range columns {
		offset := btb1HeaderBytes + col*n*btb1F64Bytes
		for i, value := range values {
			start := offset + i*btb1F64Bytes
			binary.LittleEndian.PutUint64(data[start:start+btb1F64Bytes], math.Float64bits(value))
		}
	}
	return data
}
