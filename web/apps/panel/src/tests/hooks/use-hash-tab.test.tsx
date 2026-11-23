import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';
import { MemoryRouter, useLocation, useNavigate } from 'react-router-dom';

import { useHashTab } from '@/hooks/use-hash-tab';

function Tabs({ prefix, values }: { prefix: string; values: string[] }) {
  const [tab, setTab] = useHashTab(prefix, values, values[0]);
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
    ['/nodes/n1/cores', 'cores-', ['installed', 'catalog']],
    ['/nodes/n1/configuration', 'configuration-', ['visual', 'advanced']],
    ['/nodes/n1/configuration', 'configuration-visual/', ['log', 'route']],
    ['/settings', 'panel-', ['service', 'interface']],
    ['/subscriptions', 'subscription-', ['sources', 'channels']],
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
  it('falls back for unknown hashes', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={['/settings?keep=1#unknown']}>
        <Tabs prefix='panel-' values={['service', 'backup']} />
      </MemoryRouter>,
    );
    expect(screen.getByLabelText('Current tab')).toHaveTextContent('service');
    await user.click(screen.getByRole('button', { name: 'backup' }));
    expect(screen.getByLabelText('Location')).toHaveTextContent('/settings?keep=1#panel-backup');
  });
  it('reads the visual parent tab from a nested configuration link', () => {
    render(
      <MemoryRouter initialEntries={['/nodes/n1/configuration#configuration-visual/route']}>
        <Tabs prefix='configuration-' values={['visual', 'advanced']} />
      </MemoryRouter>,
    );
    expect(screen.getByLabelText('Current tab')).toHaveTextContent('visual');
  });
});
