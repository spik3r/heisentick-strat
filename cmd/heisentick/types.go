package main

import "github.com/spik3r/heisentick-strat/report"

type gridVariant struct {
	Index  int                `json:"index"`
	Params map[string]float64 `json:"params"`
	Costs  []report.CostRow   `json:"costs"`
}

type gridPayload struct {
	Symbols  []string             `json:"symbols"`
	TFs      []string             `json:"tfs"`
	Strategy string               `json:"strategy"`
	Range    string               `json:"range"`
	Bars     int                  `json:"bars"`
	HTF      *string              `json:"htf,omitempty"`
	Sets     map[string][]float64 `json:"sets"`
	Variants []gridVariant        `json:"variants"`
	Warnings []string             `json:"warnings"`
	Options  map[string]string    `json:"options,omitempty"`
}
