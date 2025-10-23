package actioner

import "time"

type Actioner struct {
	lastCallTime time.Time
	threshold    time.Duration
}

func NewActioner(threshold int, thresholdDimensionType time.Duration) *Actioner {
	return &Actioner{time.Time{}, time.Duration(threshold) * thresholdDimensionType}
}

func (act *Actioner) ThresholdOut(now time.Time) bool {
	return now.Sub(act.lastCallTime) >= act.threshold
}

func (act *Actioner) SetLastCallTime(now time.Time) {
	act.lastCallTime = now
}
