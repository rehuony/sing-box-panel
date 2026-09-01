import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';

import '@/i18n';

import type { Task } from '@/api/api-client';

import { TasksPage } from '@/pages/tasks-page/tasks-page';
import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient, testTask } from '@/tests/api/mock-api-client';

describe('tasksPage', () => {
  it('presents accepted work as a durable queue and expands exact task details', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    render(<ApiClientProvider client={client}><TasksPage /></ApiClientProvider>);

    expect(await screen.findByRole('heading', { name: 'Task History' })).toBeInTheDocument();
    const taskSummary = screen.getByRole('button', { name: /catalog-refresh/ });

    await user.click(taskSummary);

    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(client.getTask).toHaveBeenCalledWith(testTask.id, expect.any(AbortSignal));
    expect(screen.getByText('Generation')).toBeInTheDocument();
    expect(screen.getByText('Terminal tasks cannot be canceled.')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Close' }));
    await user.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() => expect(client.listTasks).toHaveBeenCalledTimes(2));
    expect(client.getTask).toHaveBeenCalledTimes(1);
    expect(client.listTasks).toHaveBeenCalledTimes(2);
  });

  it('reports cooperative cancellation as pending until the worker finishes', async () => {
    const user = userEvent.setup();
    const runningTask: Task = {
      ...testTask,
      status: 'running',
    };
    const canceledTask: Task = {
      ...runningTask,
      cancel_requested: true,
    };
    const listTasks = vi.fn()
      .mockResolvedValueOnce({ items: [runningTask] })
      .mockResolvedValue({ items: [canceledTask] });
    const cancelTask = vi.fn().mockResolvedValue(canceledTask);
    const getTask = vi.fn().mockResolvedValue(runningTask);
    const client = createMockApiClient({ cancelTask, getTask, listTasks });
    render(<ApiClientProvider client={client}><TasksPage /></ApiClientProvider>);

    await user.click(await screen.findByRole('button', { name: /catalog-refresh/ }));
    await user.click(screen.getByRole('button', { name: 'Request cancellation' }));

    const keepTask = screen.getByRole('button', { name: 'Keep task' });
    expect(keepTask).toHaveFocus();
    await user.click(screen.getByRole('button', { name: 'Confirm cancellation' }));

    expect(cancelTask).toHaveBeenCalledWith(runningTask.id, expect.any(AbortSignal));
    expect(await screen.findByText(
      'Cancellation requested. The worker will report the final state.',
    )).toBeInTheDocument();
    expect(screen.getByText('Cancellation is already pending with the worker.'))
      .toBeInTheDocument();
  });

  it('does not reopen a task sheet when refresh finishes after the operator closed it', async () => {
    const user = userEvent.setup();
    let finishRefresh: ((page: { items: Task[] }) => void) | undefined;
    const refreshPage = new Promise<{ items: Task[] }>((resolve) => {
      finishRefresh = resolve;
    });
    const listTasks = vi.fn()
      .mockResolvedValueOnce({ items: [testTask] })
      .mockReturnValueOnce(refreshPage);
    const getTask = vi.fn().mockResolvedValue(testTask);
    const client = createMockApiClient({ getTask, listTasks });
    render(<ApiClientProvider client={client}><TasksPage /></ApiClientProvider>);

    const refresh = screen.getByRole('button', { name: 'Refresh' });
    await user.click(await screen.findByRole('button', { name: /catalog-refresh/ }));
    await screen.findByRole('dialog');
    fireEvent.click(refresh);
    await user.click(screen.getByRole('button', { name: 'Close' }));
    finishRefresh?.({ items: [testTask] });

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(getTask).toHaveBeenCalledTimes(1);
  });

  it('ignores late cancel success and failure after closing and reopening the same task', async () => {
    const user = userEvent.setup();
    const runningTask: Task = {
      ...testTask,
      status: 'running',
    };
    const canceledTask: Task = {
      ...runningTask,
      cancel_requested: true,
    };
    let resolveFirstCancel: ((task: Task) => void) | undefined;
    let rejectSecondCancel: ((reason: unknown) => void) | undefined;
    const firstCancel = new Promise<Task>((resolve) => {
      resolveFirstCancel = resolve;
    });
    const secondCancel = new Promise<Task>((_resolve, reject) => {
      rejectSecondCancel = reject;
    });
    const cancelTask = vi.fn()
      .mockReturnValueOnce(firstCancel)
      .mockReturnValueOnce(secondCancel);
    const client = createMockApiClient({
      cancelTask,
      getTask: vi.fn().mockResolvedValue(runningTask),
      listTasks: vi.fn().mockResolvedValue({ items: [runningTask] }),
    });
    render(<ApiClientProvider client={client}><TasksPage /></ApiClientProvider>);

    const openTask = async () => {
      await user.click(await screen.findByRole('button', { name: /catalog-refresh/ }));
      expect(await screen.findByRole('dialog')).toBeInTheDocument();
    };
    const startCancel = async () => {
      await user.click(screen.getByRole('button', { name: 'Request cancellation' }));
      await user.click(screen.getByRole('button', { name: 'Confirm cancellation' }));
    };

    await openTask();
    await startCancel();
    const firstSignal = cancelTask.mock.calls[0]?.[1] as AbortSignal;
    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(firstSignal.aborted).toBe(true);
    await openTask();
    await act(async () => {
      resolveFirstCancel?.(canceledTask);
      await Promise.resolve();
    });
    expect(screen.getByRole('button', { name: 'Request cancellation' })).toBeEnabled();
    expect(
      screen.queryByText('Cancellation requested. The worker will report the final state.'),
    ).not.toBeInTheDocument();

    await startCancel();
    const secondSignal = cancelTask.mock.calls[1]?.[1] as AbortSignal;
    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(secondSignal.aborted).toBe(true);
    await openTask();
    await act(async () => {
      rejectSecondCancel?.(new Error('late cancellation failure'));
      await Promise.resolve();
    });
    expect(screen.getByRole('button', { name: 'Request cancellation' })).toBeEnabled();
    expect(screen.queryByText('late cancellation failure')).not.toBeInTheDocument();
  });

  it('appends older pages without replacing the loaded queue', async () => {
    const user = userEvent.setup();
    const olderTask: Task = {
      ...testTask,
      id: 'task_older_core_install',
      kind: 'core-install',
      created_at: '2026-08-25T06:00:00Z',
      updated_at: '2026-08-25T06:02:00Z',
    };
    const cursor = { created_at: testTask.created_at, id: testTask.id };
    const listTasks = vi.fn()
      .mockResolvedValueOnce({ items: [testTask], next: cursor })
      .mockResolvedValueOnce({ items: [olderTask] });
    const client = createMockApiClient({ listTasks });
    render(<ApiClientProvider client={client}><TasksPage /></ApiClientProvider>);

    expect(await screen.findByRole('button', { name: /catalog-refresh/ })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Load older tasks' }));

    expect(await screen.findByRole('button', { name: /core-install/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /catalog-refresh/ })).toBeInTheDocument();
    expect(listTasks).toHaveBeenNthCalledWith(2, {
      beforeID: testTask.id,
      beforeTime: testTask.created_at,
      kind: undefined,
      lane: undefined,
      limit: 100,
      status: undefined,
    }, undefined);
  });

  it('does not present stale tasks as results for a failed filter request', async () => {
    const user = userEvent.setup();
    const listTasks = vi.fn()
      .mockResolvedValueOnce({ items: [testTask] })
      .mockRejectedValueOnce(new Error('filtered request failed'));
    const client = createMockApiClient({ listTasks });
    render(<ApiClientProvider client={client}><TasksPage /></ApiClientProvider>);

    expect(await screen.findByRole('button', { name: /catalog-refresh/ })).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText('Status'), 'failed');

    expect(await screen.findByText('filtered request failed')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /catalog-refresh/ })).not.toBeInTheDocument();
  });
});
