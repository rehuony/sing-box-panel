// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

type RuntimeTransition struct {
	ID                 int64                        `json:"id"`
	State              store.RuntimeTransitionState `json:"state"`
	Reason             string                       `json:"reason"`
	ActivationBundleID string                       `json:"activation_bundle_id,omitempty"`
	Generation         int64                        `json:"generation,omitempty"`
	PID                int                          `json:"pid,omitempty"`
	ProcessStartedAt   *time.Time                   `json:"process_started_at,omitempty"`
	OccurredAt         time.Time                    `json:"occurred_at"`
	UncertainSince     *time.Time                   `json:"uncertain_since,omitempty"`
}

type RuntimeTransitionCursor struct {
	OccurredAt time.Time `json:"occurred_at"`
	ID         int64     `json:"id"`
}

type RuntimeHistoryRequest struct {
	From               *time.Time
	To                 *time.Time
	State              store.RuntimeTransitionState
	Reason             string
	ActivationBundleID string
	Cursor             *store.RuntimeTransitionCursor
	Limit              int
}

type RuntimeHistoryPage struct {
	Items            []RuntimeTransition      `json:"items"`
	Next             *RuntimeTransitionCursor `json:"next,omitempty"`
	Preceding        *RuntimeTransition       `json:"preceding,omitempty"`
	HistoryStartedAt time.Time                `json:"history_started_at"`
}

func (application *Application) RuntimeHistory(
	ctx context.Context,
	request RuntimeHistoryRequest,
) (RuntimeHistoryPage, error) {
	page, err := application.database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{
		From:               request.From,
		To:                 request.To,
		State:              request.State,
		Reason:             request.Reason,
		ActivationBundleID: request.ActivationBundleID,
		Cursor:             request.Cursor,
		Limit:              request.Limit,
	})
	if err != nil {
		return RuntimeHistoryPage{}, err
	}
	result := RuntimeHistoryPage{
		Items:            make([]RuntimeTransition, len(page.Items)),
		HistoryStartedAt: page.HistoryStartedAt,
	}
	for index, transition := range page.Items {
		result.Items[index] = applicationRuntimeTransition(transition)
	}
	if page.Next != nil {
		result.Next = &RuntimeTransitionCursor{OccurredAt: page.Next.OccurredAt, ID: page.Next.ID}
	}
	if page.Preceding != nil {
		preceding := applicationRuntimeTransition(*page.Preceding)
		result.Preceding = &preceding
	}
	return result, nil
}

func applicationRuntimeTransition(value store.RuntimeTransition) RuntimeTransition {
	return RuntimeTransition{
		ID:                 value.ID,
		State:              value.State,
		Reason:             value.Reason,
		ActivationBundleID: value.ActivationBundleID,
		Generation:         value.Generation,
		PID:                value.PID,
		ProcessStartedAt:   cloneTime(value.ProcessStartedAt),
		OccurredAt:         value.OccurredAt,
		UncertainSince:     cloneTime(value.UncertainSince),
	}
}
