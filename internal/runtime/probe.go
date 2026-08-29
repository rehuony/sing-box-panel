// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"time"
)

type processOnlyProbe struct {
	clock  Clock
	window time.Duration
}

func (probe processOnlyProbe) Level() MonitoringLevel {
	return MonitoringProcessOnly
}

func (probe processOnlyProbe) AwaitHealthy(
	ctx context.Context,
	process ProcessInfo,
) (HealthObservation, error) {
	select {
	case <-process.Exited:
		return HealthObservation{}, ErrProcessExited
	default:
	}

	timer := probe.clock.NewTimer(probe.window)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return HealthObservation{}, ctx.Err()
	case <-process.Exited:
		return HealthObservation{}, ErrProcessExited
	case <-timer.C():
		select {
		case <-process.Exited:
			return HealthObservation{}, ErrProcessExited
		default:
			return HealthObservation{Healthy: true, Code: "process_alive"}, nil
		}
	}
}

func validMonitoringLevel(level MonitoringLevel) bool {
	switch level {
	case MonitoringLimited, MonitoringProcessOnly:
		return true
	default:
		return false
	}
}

func validateObservation(observation HealthObservation) error {
	if !observation.Healthy {
		return ErrHealthFailed
	}
	if !validCode(observation.Code) {
		return errors.New("health observation code is invalid")
	}
	return nil
}

func validCode(value string) bool {
	if len(value) == 0 || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') || character == '_' {
			continue
		}
		return false
	}
	return true
}
