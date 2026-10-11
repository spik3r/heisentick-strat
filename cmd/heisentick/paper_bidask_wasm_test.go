//go:build js && wasm

package main

import (
	"bytes"
	"errors"
	"testing"
)

func TestPaperBidAskWASMNativeCommandRefusal(t *testing.T) {
	const want = "{\"schema\":\"heisentick-paper-bidask-v1\",\"status\":\"refused\",\"code\":\"PAPER_BIDASK_NATIVE_COMMAND_UNAVAILABLE\"}\n"
	for _, args := range [][]string{
		{"paper-bidask"},
		{"paper-bidask", "--request-file=/private/source.json"},
		{"paper-bidask", "--request-file=/dev/stdin"},
		{"paper-bidask", "--secret=PRIVATE_PROVIDER_BODY"},
	} {
		var out bytes.Buffer
		if err := run(args, &out); err != nil || out.String() != want {
			t.Fatalf("native-only command must refuse without interpreting input: %v %q", err, out.String())
		}
	}
}

type paperBidAskWASMOutputFailure struct{ short bool }

func (w paperBidAskWASMOutputFailure) Write(b []byte) (int, error) {
	if w.short {
		return len(b) - 1, nil
	}
	return 0, errors.New("PRIVATE_OUTPUT_DETAIL")
}

func TestPaperBidAskWASMOutputFailure(t *testing.T) {
	for _, short := range []bool{false, true} {
		if err := runPaperBidAsk(nil, paperBidAskWASMOutputFailure{short}); err == nil || err.Error() != "PAPER_BIDASK_OUTPUT" {
			t.Fatalf("unsafe output failure: %v", err)
		}
	}
}
