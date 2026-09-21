// SPDX-License-Identifier: GPL-3.0-or-later
package store

// RuntimeIntent exists only for the duration of one serialized control request.
// The hub generation fences commits; it is never stored in a work queue.
type RuntimeIntent struct {
	Kind                RuntimeIntentKind
	Generation          int64
	CanonicalRevisionID string
	StartupArtifactID   string
	ActivationBundleID  string
	SelectOnly          bool
	DisableCoreID       string
	Recovery            *RuntimeRecoveryMetadata
}

// RuntimeCommit atomically binds process identity and append-only lifecycle evidence.
type RuntimeCommit struct {
	ExpectedObservation *RuntimeObservation
	Observation         *RuntimeObservation
	ClearObservation    bool
	Transitions         []RuntimeTransitionInput
}
