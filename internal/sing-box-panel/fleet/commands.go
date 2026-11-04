// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
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
// the command's deadline passes.
func (service *Service) run(ctx context.Context, nodeID string, body nodeapi.CommandBody) (*nodeapi.Result, error) {
	current := service.sessions.get(nodeID)
	if current == nil {
		return nil, ErrNodeOffline
	}
	requestID, err := database.NewID(service.random, "request")
	if err != nil {
		return nil, err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = service.now().Add(DefaultCommandTimeout)
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, deadline)
		defer cancel()
	}
	waiting := current.expect(requestID)
	defer current.forget(requestID)
	if err := current.send(&nodeapi.ConnectResponse{Frame: &nodeapi.ConnectResponse_Command{Command: &nodeapi.Command{
		RequestId: requestID, Deadline: timestamppb.New(deadline), Body: body,
	}}}); err != nil {
		return nil, ErrNodeResultUnknown
	}
	select {
	case result := <-waiting:
		if problem := result.GetProblem(); problem != nil {
			return nil, &CommandProblem{Code: problem.GetCode(), Title: problem.GetTitle(), Detail: problem.GetDetail()}
		}
		return result, nil
	case <-current.done:
		return nil, ErrNodeResultUnknown
	case <-ctx.Done():
		_ = current.send(&nodeapi.ConnectResponse{Frame: &nodeapi.ConnectResponse_Cancel{Cancel: &nodeapi.Cancel{RequestId: requestID}}})
		return nil, ctx.Err()
	}
}

// ClassifyOperationFailure names node command failures that are safe to
// record in operation outcomes: the node's own problem descriptions and the
// session failures of the panel.
func ClassifyOperationFailure(err error) (string, string, bool) {
	var problem *CommandProblem
	switch {
	case errors.As(err, &problem):
		return problem.Code, problem.Detail, true
	case errors.Is(err, ErrNodeOffline):
		return "node_offline", ErrNodeOffline.Error(), true
	case errors.Is(err, ErrNodeResultUnknown):
		return "node_result_unknown", ErrNodeResultUnknown.Error(), true
	}
	return "", "", false
}
