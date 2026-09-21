import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom';

import { useHashTab } from '@/hooks/use-hash-tab';

function Tabs({ prefix, values, clear = [] }: { prefix: string; values: string[]; clear?: string[] }) {
  const [tab, setTab] = useHashTab(prefix, values, values[0], clear);
  const location = useLocation();
  const navigate = useNavigate();
  return (
    <>
      <output aria-label='Current tab'>{tab}</output>
      <output aria-label='Location'>
        {location.pathname}
        {location.search}
        {location.hash}
      </output>
      {values.map((value) => <button key={value} onClick={() => setTab(value)}>{value}</button>)}
      <button onClick={() => void navigate(-1)}>Back</button>
      <button onClick={() => void navigate(1)}>Forward</button>
    </>
  );
}

describe('hash tab navigation', () => {
  it.each([
    ['/cores', 'cores-', ['installed', 'catalog']],
    ['/configuration', 'configuration-', ['visual', 'advanced']],
    ['/configuration', 'configuration-visual/', ['log', 'route']],
    ['/panel', 'panel-', ['security', 'appearance']],
    ['/observability', 'logs-', ['core', 'panel']],
  ] as const)('restores deep links and history on %s (%s)', async (path, prefix, values) => {
    const user = userEvent.setup();
    const url = `${path}?keep=1#${prefix}${values[1]}`;
    render(<MemoryRouter initialEntries={[url]}><Tabs prefix={prefix} values={[...values]} /></MemoryRouter>);
    expect(screen.getByLabelText('Current tab')).toHaveTextContent(values[1]);
    await user.click(screen.getByRole('button', { name: values[0] }));
    expect(screen.getByLabelText('Location')).toHaveTextContent(`${path}?keep=1#${prefix}${values[0]}`);
    await user.click(screen.getByRole('button', { name: 'Back' }));
    expect(screen.getByLabelText('Location')).toHaveTextContent(url);
    expect(screen.getByLabelText('Current tab')).toHaveTextContent(values[1]);
    await user.click(screen.getByRole('button', { name: 'Forward' }));
    expect(screen.getByLabelText('Current tab')).toHaveTextContent(values[0]);
  });
  it('falls back for unknown hashes and removes only obsolete log query fields', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={['/observability?tab=panel&task=abc&keep=1#unknown']}>
        <Tabs prefix='logs-' values={['core', 'panel']} clear={['tab', 'task']} />
      </MemoryRouter>,
    );
    expect(screen.getByLabelText('Current tab')).toHaveTextContent('core');
    await user.click(screen.getByRole('button', { name: 'panel' }));
    expect(screen.getByLabelText('Location')).toHaveTextContent('/observability?keep=1#logs-panel');
  });
  it('reads the visual parent tab from a nested configuration link', () => {
    render(
      <MemoryRouter initialEntries={['/configuration#configuration-visual/route']}>
        <Tabs prefix='configuration-' values={['visual', 'advanced']} />
      </MemoryRouter>,
    );
    expect(screen.getByLabelText('Current tab')).toHaveTextContent('visual');
  });
});
