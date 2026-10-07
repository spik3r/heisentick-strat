//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/spik3r/heisentick-strat/dsl"
	"github.com/spik3r/heisentick-strat/report/adaptiveflag"
	"github.com/spik3r/heisentick-strat/report/adaptiveflagunit"
)

func init() {
	js.Global().Set("engineRunAdaptiveFlagUnitReport", js.FuncOf(adaptiveFlagUnitReportJS))
}

func adaptiveFlagUnitReportJS(_ js.Value, args []js.Value) (response any) {
	defer func() {
		if recover() != nil {
			// Inaccessible BTB1 values are already handled by the unchanged
			// copier. Any unexpected panic has a static, payload-free fallback.
			response = adaptiveflagunit.ErrorJSON(nil)
		}
	}()
	if len(args) != 4 {
		return adaptiveflagunit.ErrorJSON(adaptiveflagunit.Rejection("projection_request_rejected", "transport", "expected exactly raw metadata JSON, projection metadata JSON, DSL source and a Uint8Array BTB1 view"))
	}
	rawMetadata, err := adaptiveFlagBoundedJSString(args[0], adaptiveflag.MaxMetadataBytes, "request")
	if err != nil {
		return adaptiveflagunit.ErrorJSON(err)
	}
	projectionMetadata, err := adaptiveFlagBoundedJSString(args[1], adaptiveflagunit.MaxProjectionMetadataBytes, "request")
	if err != nil {
		return adaptiveflagunit.ErrorJSON(adaptiveflagunit.ProjectionMetadataTransportError(err))
	}
	source, err := adaptiveFlagBoundedJSString(args[2], dsl.AdaptiveFlagRuntimeMaxSourceBytes, "source")
	if err != nil {
		return adaptiveflagunit.ErrorJSON(err)
	}
	// Strict admission of both metadata strings, source diagnostics, family,
	// config and timeframe always precedes BTB1 header access or full copy.
	prepared, err := adaptiveflagunit.PrepareRuntime(rawMetadata, projectionMetadata, source)
	if err != nil {
		return adaptiveflagunit.ErrorJSON(err)
	}
	data, err := copyAdaptiveFlagBTB1(args[3])
	if err != nil {
		return adaptiveflagunit.ErrorJSON(err)
	}
	out, err := prepared.Build(data)
	if err != nil {
		return adaptiveflagunit.ErrorJSON(err)
	}
	return string(out)
}
