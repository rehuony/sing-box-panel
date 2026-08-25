// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import "time"

type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

type Timer interface {
	C() <-chan time.Time
	Stop()
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

func (systemClock) NewTimer(duration time.Duration) Timer {
	return systemTimer{timer: time.NewTimer(duration)}
}

type systemTimer struct {
	timer *time.Timer
}

func (timer systemTimer) C() <-chan time.Time { return timer.timer.C }

func (timer systemTimer) Stop() { timer.timer.Stop() }
