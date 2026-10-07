package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spik3r/heisentick-strat/engine/master"
	"github.com/spik3r/heisentick-strat/report/masterstructural"
)

func runMasterPortableReport(args []string, out io.Writer) error {
	flags, err := parseTimedReportFlags(args)
	if err != nil {
		return err
	}
	allowed := map[string]bool{"dsl-file": true, "m5-file": true, "warmup-from": true, "trade-from": true, "trade-to": true, "spread": true, "arithmetic-contract": true}
	for key, values := range flags {
		if !allowed[key] || len(values) != 1 {
			return fmt.Errorf("master-portable-report rejects unknown/repeated --%s", key)
		}
	}
	for key := range allowed {
		if _, err = flags.required(key); err != nil {
			return err
		}
	}
	if flags.one("arithmetic-contract", "") != master.PortableArithmeticContract {
		return fmt.Errorf("master-portable-report requires --arithmetic-contract=%s", master.PortableArithmeticContract)
	}
	spread := flags.one("spread", "")
	if _, err = masterstructural.DecodePortableSpread(spread); err != nil {
		return err
	}
	var endpoints [3]int64
	for i, key := range []string{"warmup-from", "trade-from", "trade-to"} {
		raw := flags.one(key, "")
		stamp, e := time.Parse(time.RFC3339, raw)
		if e != nil || stamp.Location() != time.UTC || stamp.Format(time.RFC3339) != raw {
			return fmt.Errorf("master portable --%s requires exact UTC RFC3339 ending Z", key)
		}
		endpoints[i] = stamp.UnixMilli()
	}
	// Keep the original spread token: the shared JSON admission must never see
	// only a rounded ParseFloat replacement for an inadmissible decimal.
	metadata := fmt.Sprintf(`{"schema":%q,"arithmeticContract":%q,"warmupFromT":%d,"tradeFromT":%d,"tradeToT":%d,"spread":%s}`, masterstructural.RuntimeSchema, master.PortableArithmeticContract, endpoints[0], endpoints[1], endpoints[2], spread)
	if _, err = masterstructural.DecodeRuntimeRequest(metadata); err != nil {
		return err
	}
	source, err := readPortableFile(flags.one("dsl-file", ""), masterstructural.MaxSourceBytes)
	if err != nil {
		return err
	}
	data, err := readPortableFile(flags.one("m5-file", ""), masterstructural.MaxInputBytes)
	if err != nil {
		return err
	}
	raw, err := masterstructural.BuildPortableV1(metadata, string(source), data)
	if err != nil {
		return err
	}
	_, err = out.Write(raw)
	return err
}

func readPortableFile(path string, max int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readPortableBounded(file, max)
}

func readPortableBounded(reader io.Reader, max int) ([]byte, error) {
	// A stat check is insufficient for streams or a concurrently growing file.
	// LimitReader caps bytes consumed before ReadAll grows its bounded buffer.
	data, err := io.ReadAll(io.LimitReader(reader, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > max {
		return nil, fmt.Errorf("master portable input exceeds %d bytes", max)
	}
	return data, nil
}
