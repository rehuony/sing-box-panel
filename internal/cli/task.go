// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

func newDurableTaskCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("task", "Inspect and control durable tasks")
	root.AddCommand(
		newTaskListCommand(state, open),
		newTaskShowCommand(state, open),
		newTaskWaitCommand(state, open),
		newTaskCancelCommand(state, open),
	)
	return root
}

func newTaskListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var lane, status, kind string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List durable tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			page, err := instance.ListTasks(cmd.Context(), application.TaskListFilter{
				Lane:   store.TaskLane(strings.TrimSpace(lane)),
				Status: store.TaskStatus(strings.TrimSpace(status)),
				Kind:   store.TaskKind(strings.TrimSpace(kind)),
				Limit:  limit,
			})
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "task_filter_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, page, taskPageText(page))
		},
	}
	command.Flags().StringVar(&lane, "lane", "", "filter by runtime or maintenance lane")
	command.Flags().StringVar(&status, "status", "", "filter by task status")
	command.Flags().StringVar(&kind, "kind", "", "filter by exact task kind")
	command.Flags().IntVar(&limit, "limit", 50, "maximum tasks to return (1-200)")
	return command
}

func newTaskShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show ID",
		Short: "Show one durable task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			task, err := instance.Task(cmd.Context(), args[0])
			if err != nil {
				return classifyTaskError("task_read_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, task, taskText(task))
		},
	}
}

func newTaskCancelCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "cancel ID",
		Short: "Cancel queued work or request cancellation at a running task's next safe boundary",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			task, err := instance.CancelTask(cmd.Context(), args[0])
			if err != nil {
				return classifyTaskError("task_cancel_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, task, taskText(task))
		},
	}
}

func newTaskWaitCommand(state *options, open openApplicationFunc) *cobra.Command {
	var pollInterval time.Duration
	command := &cobra.Command{
		Use:   "wait ID",
		Short: "Wait until a durable task reaches a terminal state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if pollInterval < 50*time.Millisecond || pollInterval > 10*time.Second {
				return &Error{Kind: ErrorUsage, Code: "poll_interval_invalid", Message: "--poll-interval must be between 50ms and 10s"}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			task, err := waitForTaskWithCancellationRequest(
				cmd.Context(), instance, args[0], pollInterval, "task_wait_failed",
			)
			if err != nil {
				return err
			}
			if err := writeResult(cmd.OutOrStdout(), state.format, task, taskText(task)); err != nil {
				return err
			}
			return terminalTaskError(task)
		},
	}
	command.Flags().DurationVar(&pollInterval, "poll-interval", 250*time.Millisecond, "local SQLite polling interval")
	return command
}

func waitForTaskWithCancellationRequest(
	ctx context.Context,
	instance *application.Application,
	taskID string,
	pollInterval time.Duration,
	fallbackCode string,
) (application.Task, error) {
	task, err := waitForTask(ctx, instance, taskID, pollInterval)
	if err == nil {
		return task, nil
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return application.Task{}, classifyTaskError(fallbackCode, err)
	}

	cancelContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_, cancelErr := instance.CancelTask(cancelContext, taskID)
	message := fmt.Sprintf("wait interrupted; cancellation requested for task %s", taskID)
	if cancelErr != nil {
		message = fmt.Sprintf("wait interrupted; task %s cancellation request failed: %v", taskID, cancelErr)
	}
	return application.Task{}, &Error{
		Kind: ErrorDomain, Code: "task_wait_interrupted", Message: message, Cause: err,
	}
}

func waitForTask(
	ctx context.Context,
	instance *application.Application,
	taskID string,
	pollInterval time.Duration,
) (application.Task, error) {
	for {
		task, err := instance.Task(ctx, taskID)
		if err != nil {
			return application.Task{}, err
		}
		if task.Terminal() {
			return task, nil
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return application.Task{}, context.Cause(ctx)
		case <-timer.C:
		}
	}
}

func classifyTaskError(code string, err error) error {
	if application.IsTaskNotFound(err) {
		return &Error{Kind: ErrorDomain, Code: "task_not_found", Message: err.Error(), Cause: err}
	}
	return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
}

func taskPageText(page application.TaskPage) string {
	if len(page.Items) == 0 {
		return "no tasks"
	}
	var output strings.Builder
	for _, task := range page.Items {
		fmt.Fprintf(&output, "%s\t%s\t%s\t%s\n", task.ID, task.Lane, task.Status, task.Kind)
	}
	return strings.TrimSuffix(output.String(), "\n")
}

func taskText(task application.Task) string {
	return fmt.Sprintf("%s\t%s\t%s\t%s", task.ID, task.Lane, task.Status, task.Kind)
}

func taskFailureMessage(task application.Task) string {
	if len(task.Failure) == 0 {
		return "task failed: " + task.ID
	}
	return fmt.Sprintf("task failed: %s: %s", task.ID, strings.TrimSpace(string(task.Failure)))
}

func terminalTaskError(task application.Task) error {
	switch task.Status {
	case store.TaskStatusSucceeded:
		return nil
	case store.TaskStatusFailed:
		return &Error{Kind: ErrorDomain, Code: "task_failed", Message: taskFailureMessage(task)}
	case store.TaskStatusCanceled:
		return &Error{Kind: ErrorConflict, Code: "task_canceled", Message: "task was canceled: " + task.ID}
	case store.TaskStatusSuperseded:
		return &Error{Kind: ErrorConflict, Code: "task_superseded", Message: "task was superseded: " + task.ID}
	default:
		return &Error{Kind: ErrorDomain, Code: "task_state_invalid", Message: "task returned a non-terminal state"}
	}
}
