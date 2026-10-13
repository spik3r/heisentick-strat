//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/spik3r/heisentick-strat/engine"
	"github.com/spik3r/heisentick-strat/internal/sourcetext"
)

// Two additive exports taking (fixtureJSON, source, calendarJSON): exactly three
// strings. calendarJSON is the empty string when no closure calendar is supplied.
func registerSequentialMarketV3() {
	js.Global().Set("engineRunSequentialMarketV3", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return sequentialMarketV3Export(args, runSequentialMarketV3, sequentialMarketV3Failure)
	}))
	js.Global().Set("engineAdmitSequentialMarketV3", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return sequentialMarketV3Export(args, admitSequentialMarketV3, sequentialMarketV3AdmissionFailure)
	}))
}

func sequentialMarketV3Export(args []js.Value, run func(raw, source, calendar string) ([]byte, bool), fail func(error) []byte) any {
	if len(args) != 3 || !sequentialMarketStringArgument(args[0]) || !sequentialMarketStringArgument(args[1]) || !sequentialMarketStringArgument(args[2]) {
		return string(fail(sequentialInvalid("arguments", "expected fixture JSON, source and calendar JSON strings")))
	}
	length := func(value js.Value) int { return js.Global().Get("Object").Invoke(value).Length() }
	if length(args[0]) > sequentialMaxFixture {
		return string(fail(sequentialInvalid("fixture", "fixture JSON exceeds 64 MiB")))
	}
	if length(args[1]) > sequentialMaxSource {
		return string(fail(&sequentialInputError{"SEQUENTIAL_DSL_INVALID", "source", sequentialSourceLimitMessage}))
	}
	if length(args[2]) > engine.SequentialCalendarMaxBytes {
		return string(fail(&engine.SequentialCalendarError{Kind: "invalid-sequential-calendar", Field: "calendar.size", Index: -1}))
	}
	source, err := sourcetext.FromJS(args[1])
	if err != nil {
		return string(fail(&sequentialInputError{"SEQUENTIAL_DSL_INVALID", "source", err.Error()}))
	}
	raw, err := sourcetext.FromJS(args[0])
	if err != nil {
		return string(fail(sequentialInvalid("fixture", "fixture JSON must contain valid UTF-16")))
	}
	calendar, err := sourcetext.FromJS(args[2])
	if err != nil {
		return string(fail(sequentialInvalid("calendar", "calendar JSON must contain valid UTF-16")))
	}
	out, _ := run(raw, source, calendar)
	return string(out)
}
