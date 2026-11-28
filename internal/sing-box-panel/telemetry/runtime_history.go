// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
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
	NodeID             string
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

func (application *Service) RuntimeHistory(
	ctx context.Context,
	request RuntimeHistoryRequest,
) (RuntimeHistoryPage, error) {
	page, err := application.database.ListRuntimeTransitions(ctx, store.RuntimeHistoryFilter{
		NodeID:             request.NodeID,
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

// NodeRuntimeTransition is one runtime transition with the node it belongs to.
type NodeRuntimeTransition struct {
	NodeID   string `json:"node_id"`
	NodeName string `json:"node_name"`
	RuntimeTransition
}

type FleetRuntimeHistoryPage struct {
	Items []NodeRuntimeTransition `json:"items"`
	Total int                     `json:"total"`
}

// FleetRuntimeHistory lists every node's runtime history, newest first.
func (application *Service) FleetRuntimeHistory(ctx context.Context, filter store.FleetRuntimeHistoryFilter) (FleetRuntimeHistoryPage, error) {
	page, err := application.database.ListFleetRuntimeTransitions(ctx, filter)
	if err != nil {
		return FleetRuntimeHistoryPage{}, err
	}
	result := FleetRuntimeHistoryPage{Items: make([]NodeRuntimeTransition, len(page.Items)), Total: page.Total}
	for index, item := range page.Items {
		result.Items[index] = NodeRuntimeTransition{NodeID: item.NodeID, NodeName: item.NodeName, RuntimeTransition: applicationRuntimeTransition(item.RuntimeTransition)}
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
		ProcessStartedAt:   store.CloneTime(value.ProcessStartedAt),
		OccurredAt:         value.OccurredAt,
		UncertainSince:     store.CloneTime(value.UncertainSince),
	}
}

type RuntimeHistoryClearResult struct {
	Deleted int64 `json:"deleted"`
}

// ClearRuntimeHistory deletes the node's runtime history except each of its
// instances' newest transition, the current state and where the node resumes
// replicating, makes its dashboard read every bucket again and records one
// entry about the node saying how many it deleted.
func (application *Service) ClearRuntimeHistory(ctx context.Context, nodeID string) (RuntimeHistoryClearResult, error) {
	deleted, err := application.database.ClearRuntimeTransitions(ctx, nodeID)
	if deleted > 0 {
		application.resetDashboardHistory(nodeID)
	}
	if err != nil {
		return RuntimeHistoryClearResult{}, err
	}
	application.RecordEvent(LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "runtime.history.cleared", Message: "Runtime history cleared",
		Metadata: LogMetadata(map[string]any{"node_id": nodeID, "deleted": deleted}),
	})
	return RuntimeHistoryClearResult{Deleted: deleted}, nil
}

// RecordRuntimeTransition stores one entry of a node's runtime history.
func (application *Service) RecordRuntimeTransition(ctx context.Context, input store.RuntimeTransitionInput) error {
	return application.database.RecordRuntimeTransition(ctx, input)
}

// LatestRuntimeTransitionSource returns the newest entry the panel holds of a
// node instance's history.
func (application *Service) LatestRuntimeTransitionSource(ctx context.Context, nodeID, instanceID string) (int64, error) {
	return application.database.LatestRuntimeTransitionSource(ctx, nodeID, instanceID)
}
