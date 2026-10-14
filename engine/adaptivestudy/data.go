package adaptivestudy

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/spik3r/heisentick-strat/marketdata"
)

type VerifiedM30 struct {
	bars []Bar
	hash string
}

func LoadM30(path string) (VerifiedM30, error) {
	f, e := os.Open(path)
	if e != nil {
		return VerifiedM30{}, e
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, 4<<20))
	if e != nil {
		return VerifiedM30{}, e
	}
	return decodeM30(raw)
}
func decodeM30(raw []byte) (VerifiedM30, error) {
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != M30Hash {
		return VerifiedM30{}, fmt.Errorf("M30 pinned hash mismatch")
	}
	if len(raw) != 16+67941*6*8 || binary.LittleEndian.Uint32(raw[8:12]) != 67941 || binary.LittleEndian.Uint32(raw[12:16]) != 6 {
		return VerifiedM30{}, fmt.Errorf("M30 shape mismatch")
	}
	s, e := marketdata.DecodeBTB1(raw)
	if e != nil {
		return VerifiedM30{}, e
	}
	bars := make([]Bar, len(s.T))
	for n, t := range s.T {
		if math.IsNaN(t) || math.IsInf(t, 0) || t < 0 || t > 9007199254740991 || t != math.Trunc(t) || int64(t)%Duration != 0 {
			return VerifiedM30{}, fmt.Errorf("M30 grid timestamp")
		}
		bars[n] = Bar{T: int64(t), CloseT: int64(t) + Duration, Open: s.O[n], High: s.H[n], Low: s.L[n], Close: s.C[n]}
	}
	return VerifiedM30{bars: bars, hash: M30Hash}, nil
}
func date(y int, m time.Month, d int) int64 {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).UnixMilli()
}
func StudyWindow(name string) (int64, int64, error) {
	start := date(2021, time.April, 1)
	switch name {
	case "development", "train2024":
		return start, date(2024, time.January, 1), nil
	case "2024":
		return date(2024, time.January, 1), date(2025, time.January, 1), nil
	case "2025":
		return date(2025, time.January, 1), date(2026, time.January, 1), nil
	case "2026":
		return date(2026, time.January, 1), date(2026, time.October, 1), nil
	case "train2025":
		return start, date(2025, time.January, 1), nil
	case "train2026":
		return start, date(2026, time.January, 1), nil
	}
	return 0, 0, fmt.Errorf("unknown frozen study window")
}
func errSelection(s string) error { return fmt.Errorf("selector: %s", s) }
