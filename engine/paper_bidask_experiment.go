package engine

// A calendar is an account-division attestation supplied by the authenticated
// backend. Go binds its complete dated contents, but does not certify a broker
// schedule or infer the account's legal division from a timezone.
type BidAskClosureWindow struct {
	FromT    string `json:"fromT"`
	ThroughT string `json:"throughT"`
}
type BidAskExperiment struct {
	CalendarThroughT  string                `json:"calendarThroughT"`
	Profile           string                `json:"profile"`
	AttestationDigest string                `json:"attestationDigest"`
	EntryUntilT       string                `json:"entryUntilT"`
	ObservationUntilT string                `json:"observationUntilT"`
	SampleCadenceMS   string                `json:"sampleCadenceMs"`
	Closures          []BidAskClosureWindow `json:"closures"`
}
type BidAskExperimentClock struct {
	EffectiveT string `json:"effectiveT"`
	Reason     string `json:"reason"`
}

type BidAskClosure struct {
	WindowIndex string `json:"windowIndex"`
	FromT       string `json:"fromT"`
	ThroughT    string `json:"throughT"`
}

func validateBidAskExperiment(r PaperBidAskRequest, family string) error {
	e := r.Experiment
	if e == nil {
		return nil
	}
	if family != "dualEmaResumption" || e.Profile != "oanda-au-v20-gold-dated-v1" ||
		!bidAskDigestValid(e.AttestationDigest) || e.SampleCadenceMS != "300000" || len(e.Closures) > 128 {
		return bidAskError("EXPERIMENT")
	}
	activation, _ := bidAskTime(r.ActivationFromT)
	entry, err := bidAskTime(e.EntryUntilT)
	if err != nil {
		return err
	}
	observation, err := bidAskTime(e.ObservationUntilT)
	if err != nil {
		return err
	}
	day := int64(86400000000000)
	if entry != activation+32*day || observation != entry+14*day {
		return bidAskError("EXPERIMENT_HORIZON")
	}
	calendarThrough, err := bidAskTime(e.CalendarThroughT)
	if err != nil || calendarThrough <= activation || calendarThrough > observation {
		return bidAskError("CALENDAR_HORIZON")
	}
	previous := int64(-1)
	for _, window := range e.Closures {
		from, err := bidAskTime(window.FromT)
		if err != nil {
			return err
		}
		through, err := bidAskTime(window.ThroughT)
		if err != nil {
			return err
		}
		if from < activation || from >= through || through > calendarThrough || from < previous {
			return bidAskError("CALENDAR")
		}
		previous = through
	}
	return nil
}

func (x *bidAskRunner) onClosure(rec BidAskRecord) {
	c := rec.Closure
	from, _ := bidAskTime(c.FromT)
	through, _ := bidAskTime(c.ThroughT)
	if (x.pending != nil && x.pending.episode != nil) || (x.lastBarClose != from && !(x.position == nil && x.lastBarClose < x.activation && from == x.activation)) || x.coverage != "sampled-observations-only" {
		x.err = bidAskError("CLOSURE_STATE")
		return
	}
	// Only fully closed grid intervals are omitted. A partially open interval
	// still requires its actual provider bar and keeps its actual extrema.
	x.lastBarOpen = through - x.duration
	x.lastBarClose = through
	if x.position != nil {
		x.position.view.ExpectedFullIntervalOpenT = c.ThroughT
		x.collector.account.MarkStatus = "scheduled-market-closure"
	}
	x.emit(rec, BidAskEvent{Kind: "coverage", Outcome: "scheduled-market-closure",
		Basis: "attested-no-market-interval", Clock: BidAskClock{
			IntervalOpenT: c.FromT, IntervalCloseT: c.ThroughT, ModelEffectiveT: c.ThroughT}})
}

func (x *bidAskRunner) entriesOpen(at int64) bool {
	if x.request.Experiment == nil {
		return true
	}
	until, _ := bidAskTime(x.request.Experiment.EntryUntilT)
	calendarThrough, _ := bidAskTime(x.request.Experiment.CalendarThroughT)
	return at < until && at < calendarThrough
}

func validateBidAskExperimentRecord(r PaperBidAskRequest, rec BidAskRecord) error {
	if r.Experiment == nil {
		return nil
	}
	commit, err := bidAskTime(rec.CommitT)
	if err != nil {
		return err
	}
	until, err := bidAskTime(r.Experiment.ObservationUntilT)
	if err != nil {
		return err
	}
	calendarThrough, err := bidAskTime(r.Experiment.CalendarThroughT)
	if err != nil {
		return err
	}
	if (commit > until || commit > calendarThrough) && rec.ExperimentClock == nil {
		return bidAskError("EXPERIMENT_HORIZON")
	}
	for _, window := range r.Experiment.Closures {
		from, err := bidAskTime(window.FromT)
		if err != nil {
			return err
		}
		through, err := bidAskTime(window.ThroughT)
		if err != nil {
			return err
		}
		if rec.Bar != nil {
			open, err := bidAskTime(rec.Bar.OpenT)
			if err != nil {
				return err
			}
			close, err := bidAskTime(rec.Bar.CloseT)
			if err != nil {
				return err
			}
			if open >= from && close <= through {
				return bidAskError("CALENDAR_CONTRADICTION")
			}
		}
		if rec.Quote != nil && rec.Quote.Tradeable {
			at, err := bidAskTime(rec.Quote.ProviderT)
			if err != nil {
				return err
			}
			if at >= from && at < through {
				return bidAskError("CALENDAR_CONTRADICTION")
			}
		}
	}
	return nil
}

func (x *bidAskRunner) modeledOpenClock(bar *BidAskBar, clock BidAskClock) (BidAskClock, string) {
	clock.ModelEffectiveT = bar.OpenT
	if x.request.Experiment != nil {
		open, _ := bidAskTime(bar.OpenT)
		close, _ := bidAskTime(bar.CloseT)
		for _, window := range x.request.Experiment.Closures {
			from, _ := bidAskTime(window.FromT)
			through, _ := bidAskTime(window.ThroughT)
			if open >= from && open < through && through < close {
				clock.ModelEffectiveT = window.ThroughT
				return clock, "modeled-side-open-after-scheduled-reopen"
			}
		}
	}
	return clock, "modeled-side-open"
}

func (x *bidAskRunner) onExperimentClock(rec BidAskRecord) {
	reason := rec.ExperimentClock.Reason
	if x.pending != nil {
		x.noFill(rec, "experiment-ended", BidAskClock{ModelEffectiveT: rec.ExperimentClock.EffectiveT})
	}
	if x.position != nil {
		x.coverage = "unresolved-exposure:" + reason
		x.collector.account.MarkStatus = "stale-unresolved"
		x.emit(rec, BidAskEvent{Kind: "coverage", Outcome: reason, Basis: "terminal-no-fabricated-liquidation", Clock: BidAskClock{ModelEffectiveT: rec.ExperimentClock.EffectiveT}})
	} else {
		x.coverage = "experiment-complete:" + reason
		x.collector.account.MarkStatus = "experiment-complete"
		x.emit(rec, BidAskEvent{Kind: "coverage", Outcome: reason, Basis: "terminal-no-fabricated-liquidation", Clock: BidAskClock{ModelEffectiveT: rec.ExperimentClock.EffectiveT}})
	}
}

func (x *bidAskRunner) signalExecutableFrom(close int64) int64 {
	if x.request.Experiment != nil {
		for _, window := range x.request.Experiment.Closures {
			from, _ := bidAskTime(window.FromT)
			through, _ := bidAskTime(window.ThroughT)
			if close >= from && close < through {
				close = through
			}
		}
	}
	return close
}
