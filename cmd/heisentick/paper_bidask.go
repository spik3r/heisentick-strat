package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/spik3r/heisentick-strat/engine"
)

// runPaperBidAsk performs bounded local file IO only, with a single complete
// success/refusal envelope. It has no feed, network, authentication or order API.
func runPaperBidAsk(args []string, out io.Writer) error {
	refusal := func(code string) error {
		raw, _ := json.Marshal(struct {
			Schema string `json:"schema"`
			Status string `json:"status"`
			Code   string `json:"code"`
		}{engine.PaperBidAskSchema, "refused", code})
		raw = append(raw, '\n')
		if n, e := out.Write(raw); e != nil || n != len(raw) {
			return fmt.Errorf("PAPER_BIDASK_OUTPUT")
		}
		return nil
	}
	if len(args) != 1 || !strings.HasPrefix(args[0], "--request-file=") || len(args[0]) == len("--request-file=") {
		return refusal("PAPER_BIDASK_ARGUMENTS")
	}
	file, e := os.OpenFile(strings.TrimPrefix(args[0], "--request-file="), os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return refusal("PAPER_BIDASK_INPUT")
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > engine.PaperBidAskMaxBytes {
		return refusal("PAPER_BIDASK_INPUT")
	}
	raw, e := io.ReadAll(io.LimitReader(file, engine.PaperBidAskMaxBytes+1))
	if e != nil {
		return refusal("PAPER_BIDASK_INPUT")
	}
	r, e := engine.DecodePaperBidAskRequest(raw)
	if e != nil {
		return refusal(e.Error())
	}
	result, e := engine.RunPaperBidAsk(r)
	if e != nil {
		return refusal(e.Error())
	}
	encoded, e := engine.EncodePaperBidAskResult(result)
	if e != nil {
		return refusal(e.Error())
	}
	encoded = append(encoded, '\n')
	if n, e := out.Write(encoded); e != nil || n != len(encoded) {
		return fmt.Errorf("PAPER_BIDASK_OUTPUT")
	}
	return nil
}
