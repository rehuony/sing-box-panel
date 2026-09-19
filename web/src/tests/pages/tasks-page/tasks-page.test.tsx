import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';

import { TasksPage } from '@/pages/tasks-page/tasks-page';

function Destination() {
  const location = useLocation();
  return <output>{location.pathname + location.search}</output>;
}
describe('legacy task link', () => {
  it('preserves the exact task ID when forwarding to panel logs', async () => {
    render(
      <MemoryRouter initialEntries={['/tasks?task=task_42']}>
        <Routes>
          <Route path='/tasks' element={<TasksPage />} />
          <Route path='/observability' element={<Destination />} />
        </Routes>
      </MemoryRouter>,
    );
    expect(await screen.findByText('/observability?tab=panel&task=task_42')).toBeVisible();
  });
});
