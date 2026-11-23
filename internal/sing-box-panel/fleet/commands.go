// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/releases"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// DefaultCommandTimeout bounds a command when the caller sets no deadline.
const DefaultCommandTimeout = 2 * time.Minute

var (
	// ErrNodeOffline rejects a command for a node without a session.
	// Commands are never queued.
	ErrNodeOffline = errors.New("the node is offline")
	// ErrNodeResultUnknown reports a command whose session ended before the
	// node answered; the node's state settles the outcome.
	ErrNodeResultUnknown = errors.New("the node's session ended before it answered")
)

// CommandProblem is a failure the node reported for a command.
type CommandProblem struct {
	Code   string
	Title  string
	Detail string
}

func (problem *CommandProblem) Error() string {
	return "node command failed: " + problem.Code + ": " + problem.Detail
}

// IsProblem reports whether err is a node problem with code.
func IsProblem(err error, code string) bool {
	var problem *CommandProblem
	return errors.As(err, &problem) && problem.Code == code
}

// run sends body to nodeID and waits for the node's result until ctx ends or
// the command's deadline passes; without a deadline on ctx the command has
// DefaultCommandTimeout. The deadline also bounds the wait for a session's
// first state report. A command that can change the node's state keeps only
// its deadline once it is sent: cancelling ctx, as a client that disconnects
// does, would otherwise interrupt a change that already started on the node.
func (service *Service) run(ctx context.Context, nodeID string, body nodeapi.CommandBody) (*nodeapi.Result, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = service.now().Add(DefaultCommandTimeout)
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	current, err := service.readySession(ctx, nodeID, commandMinor(body))
	if err != nil {
		return nil, err
	}
	if nodeapi.ChangesState(body) {
		var detached context.CancelFunc
		ctx, detached = context.WithDeadline(context.WithoutCancel(ctx), deadline)
		defer detached()
	}
	requestID, err := database.NewID(service.random, "request")
	if err != nil {
		return nil, err
	}
	waiting := current.expect(requestID)
	defer current.forget(requestID)
	if err := current.send(&nodeapi.ConnectResponse{Frame: &nodeapi.ConnectResponse_Command{Command: &nodeapi.Command{
		RequestId: requestID, Deadline: timestamppb.New(deadline), Body: body,
	}}}); err != nil {
		return nil, ErrNodeResultUnknown
	}
	answer := func(result *nodeapi.Result) (*nodeapi.Result, error) {
		if problem := result.GetProblem(); problem != nil {
			return nil, &CommandProblem{Code: problem.GetCode(), Title: problem.GetTitle(), Detail: problem.GetDetail()}
		}
		return result, nil
	}
	select {
	case result := <-waiting:
		return answer(result)
	case <-current.done:
		// A result delivered just before the session ended still counts.
		select {
		case result := <-waiting:
			return answer(result)
		default:
			return nil, ErrNodeResultUnknown
		}
	case <-ctx.Done():
		select {
		case result := <-waiting:
			return answer(result)
		default:
		}
		_ = current.send(&nodeapi.ConnectResponse{Frame: &nodeapi.ConnectResponse_Cancel{Cancel: &nodeapi.Cancel{RequestId: requestID}}})
		return nil, ctx.Err()
	}
}

// readySession returns the node's session once its first state report is
// recorded: the panel acts on a node only once it knows the node's state. A
// session replaced before it reported hands over to its successor.
func (service *Service) readySession(ctx context.Context, nodeID string, minor uint32) (*session, error) {
	for {
		current := service.sessions.get(nodeID)
		if current == nil {
			return nil, ErrNodeOffline
		}
		if current.protocolMinor < minor {
			return nil, ErrNodeUpgradeRequired
		}
		select {
		case <-current.ready:
			return current, nil
		case <-current.done:
			if next := service.sessions.get(nodeID); next != nil && next != current {
				continue
			}
			return nil, ErrNodeOffline
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// ClassifyOperationFailure names node command failures that are safe to
// record in operation outcomes: the node's own problem descriptions and the
// session failures of the panel.
func ClassifyOperationFailure(err error) (string, string, bool) {
	var problem *CommandProblem
	var update *NodeUpdateFailure
	var release *releases.Failure
	switch {
	case errors.As(err, &problem):
		return problem.Code, problem.Detail, true
	case errors.As(err, &update):
		return "node_update_failed", update.Error(), true
	case errors.As(err, &release):
		return release.Code, "The update failed with " + release.Code + ".", true
	}
	for _, known := range []struct {
		target error
		code   string
	}{
		{ErrNodeOffline, "node_offline"},
		{ErrNodeResultUnknown, "node_result_unknown"},
		{ErrNodeUpgradeRequired, "node_upgrade_required"},
		{ErrNodeRestartRequired, "node_restart_required"},
		{ErrNodeUpdateTimeout, "node_update_timeout"},
		{releases.ErrUnavailable, "node_update_unavailable"},
		{releases.ErrPlatformUnsupported, "node_platform_unsupported"},
	} {
		if errors.Is(err, known.target) {
			return known.code, known.target.Error(), true
		}
	}
	return "", "", false
}

// BatchConcurrency is how many nodes a batch command runs on at once.
const BatchConcurrency = 4

// BatchTarget selects the nodes of a batch command: NodeIDs, or every node
// with Tag.
type BatchTarget struct {
	NodeIDs []string
	Tag     string
}

// BatchNode is one node a batch command selects; Node is nil for an ID of
// no node.
type BatchNode struct {
	ID   string
	Node *Node
}

// BatchResult is one node's outcome in a batch command.
type BatchResult struct {
	NodeID string
	Err    error
}

// BatchNodes returns the nodes target selects: the listed IDs in order
// without repeats, or every node with the tag in name order.
func (service *Service) BatchNodes(ctx context.Context, target BatchTarget) ([]BatchNode, error) {
	records, err := service.database.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	known := make(map[string]*Node, len(records))
	var nodes []BatchNode
	for _, record := range records {
		view := service.view(record)
		known[record.ID] = &view
		if target.Tag != "" && slices.Contains(view.Tags, target.Tag) {
			nodes = append(nodes, BatchNode{ID: record.ID, Node: &view})
		}
	}
	if target.Tag == "" {
		seen := make(map[string]bool, len(target.NodeIDs))
		for _, nodeID := range target.NodeIDs {
			if !seen[nodeID] {
				seen[nodeID] = true
				nodes = append(nodes, BatchNode{ID: nodeID, Node: known[nodeID]})
			}
		}
	}
	return nodes, nil
}

// RunBatch runs command on each node, at most BatchConcurrency at once, and
// calls report with each node's outcome as it finishes, one call at a time.
// An ID of no node is reported at once with store.ErrNodeNotFound and does
// not run. command records each node's operation as the single-node command
// does. The batch keeps running when ctx is cancelled, so every selected
// node receives the command that was confirmed; each command keeps its own
// deadline. RunBatch returns once every node is reported.
func (service *Service) RunBatch(ctx context.Context, nodes []BatchNode, command func(context.Context, Node) error, report func(BatchResult)) {
	ctx = context.WithoutCancel(ctx)
	var reporting sync.Mutex
	finish := func(result BatchResult) {
		reporting.Lock()
		defer reporting.Unlock()
		report(result)
	}
	for _, node := range nodes {
		if node.Node == nil {
			finish(BatchResult{NodeID: node.ID, Err: store.ErrNodeNotFound})
		}
	}
	slots := make(chan struct{}, BatchConcurrency)
	var wait sync.WaitGroup
	for _, node := range nodes {
		if node.Node == nil {
			continue
		}
		slots <- struct{}{}
		wait.Go(func() {
			defer func() { <-slots }()
			finish(BatchResult{NodeID: node.ID, Err: command(ctx, *node.Node)})
		})
	}
	wait.Wait()
}
