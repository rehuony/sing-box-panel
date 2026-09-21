package server

import "time"

// runtimeClock supplies lifecycle time and tickers. Production uses the system clock;
// tests can drive every transition without sleeping.
type runtimeClock interface {
	Now() time.Time
	NewTicker(time.Duration) runtimeTicker
}

// runtimeTicker is the subset of time.Ticker owned by the reconciler.
type runtimeTicker interface {
	C() <-chan time.Time
	Stop()
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

func (systemClock) NewTicker(interval time.Duration) runtimeTicker {
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
