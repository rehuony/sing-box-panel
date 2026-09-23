import { useState } from 'react';
import { expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';

import '@/i18n';
import { ListPagination } from '@/components/list-pagination';

function PaginatedList() {
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(10);
  return (
    <ListPagination page={page} pages={Math.ceil(12 / size)} pageSize={size}
      onPageChange={setPage} onPageSizeChange={value => {
        setSize(value);
        setPage(1);
      }} />
  );
}

it('shares page entry, boundaries and totals linked to the selected page size', async () => {
  const user = userEvent.setup();
  render(<PaginatedList />);
  const input = screen.getByRole('spinbutton', { name: 'Current page' });
  expect(input).toHaveValue(1);
  expect(input).toHaveAccessibleDescription('2 pages in total');
  expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Next page' }));
  expect(input).toHaveValue(2);
  expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();
  await user.click(screen.getByRole('combobox', { name: 'Items per page' }));
  await user.click(await screen.findByRole('option', { name: '5 per page' }));
  expect(input).toHaveValue(1);
  expect(input).toHaveAccessibleDescription('3 pages in total');
  await user.clear(input);
  await user.type(input, '99{Enter}');
  expect(input).toHaveValue(3);
  await user.clear(input);
  await user.type(input, '1{Escape}');
  expect(input).toHaveValue(3);
  await user.clear(input);
  await user.type(input, '2');
  await user.tab();
  expect(input).toHaveValue(2);
  await user.clear(input);
  await user.type(input, '1.5{Enter}');
  expect(input).toHaveValue(2);
});

it('disables empty or loading navigation and synchronizes externally changed pages', () => {
  const change = vi.fn();
  const { rerender } = render(
    <ListPagination page={3} pages={4} pageSize={10} onPageChange={change} onPageSizeChange={change} />,
  );
  rerender(
    <ListPagination page={1} pages={1} pageSize={10} disabled onPageChange={change} onPageSizeChange={change} />,
  );
  expect(screen.getByRole('spinbutton')).toHaveValue(1);
  expect(screen.getByRole('spinbutton')).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();
  expect(change).not.toHaveBeenCalled();
});
