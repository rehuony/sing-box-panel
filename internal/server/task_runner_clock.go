package server

import "time"

// taskClock supplies lifecycle time and tickers. Production uses the system clock;
// tests can drive every transition without sleeping.
type taskClock interface {
	Now() time.Time
	NewTicker(time.Duration) taskTicker
}

// taskTicker is the subset of time.Ticker owned by the runner.
type taskTicker interface {
	C() <-chan time.Time
	Stop()
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

func (systemClock) NewTicker(interval time.Duration) taskTicker {
	return systemTicker{ticker: time.NewTicker(interval)}
}

type systemTicker struct {
	ticker *time.Ticker
}

func (t systemTicker) C() <-chan time.Time {
	return t.ticker.C
}

func (t systemTicker) Stop() {
	t.ticker.Stop()
}
