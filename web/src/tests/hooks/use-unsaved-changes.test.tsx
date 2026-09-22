import { useState } from 'react';
import { expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { useLocation, useNavigate } from 'react-router-dom';
import { render, screen, waitFor } from '@testing-library/react';

import '@/i18n';
import { TestRouter } from '@/tests/test-router';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';

function Editor({ allowSamePathNavigation = false, name = 'Draft' }: { allowSamePathNavigation?: boolean; name?: string }) {
  const [value, setValue] = useState('');
  const [busy, setBusy] = useState(false);
  useUnsavedChanges(value !== '', () => setValue(''), busy, { allowSamePathNavigation });
  return (
    <>
      <input aria-label={name} value={value} onChange={event => setValue(event.target.value)} />
      <button onClick={() => setValue('')}>
        {`Save ${name}`}
      </button>
      <button onClick={() => setBusy(true)}>
        {`Saving ${name}`}
      </button>
    </>
  );
}

function Navigation() {
  const navigate = useNavigate();
  const location = useLocation();
  return (
    <>
      <output data-testid='location'>
        {location.pathname}
        {location.hash}
      </output>
      <button onClick={() => void navigate('/other#tab')}>Other page</button>
      <button onClick={() => void navigate('#second')}>Hash tab</button>
      <button onClick={() => void navigate('#second', { replace: true })}>Replace hash</button>
      <button onClick={() => void navigate(-1)}>Back</button>
      <button onClick={() => void navigate(1)}>Forward</button>
    </>
  );
}

it.each(['Other page', 'Hash tab', 'Replace hash', 'Back', 'Forward'])(
  'protects edits during %s navigation and resumes the exact destination on discard', async action => {
    const user = userEvent.setup();
    render(
      <TestRouter initialEntries={['/previous#one', '/current#one', '/next#one']} initialIndex={1}>
        <Editor />
        <Navigation />
      </TestRouter>,
    );
    await user.type(screen.getByRole('textbox'), 'unsaved');
    await user.click(screen.getByRole('button', { name: action }));
    expect(screen.getByTestId('location')).toHaveTextContent('/current#one');
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(screen.getByRole('textbox')).toHaveValue('unsaved');
    await user.click(screen.getByRole('button', { name: action }));
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    const expected = action === 'Other page' ? '/other#tab' : action === 'Back' ? '/previous#one' : action === 'Forward' ? '/next#one' : '/current#second';
    expect(screen.getByTestId('location')).toHaveTextContent(expected);
    expect(screen.getByRole('textbox')).toHaveValue('');
  },
);

it.each(['Hash tab', 'Replace hash'])(
  'keeps edits without confirmation during same-page %s navigation', async action => {
    const user = userEvent.setup();
    render(
      <TestRouter initialEntries={['/configuration#one']}>
        <Editor allowSamePathNavigation />
        <Navigation />
      </TestRouter>,
    );
    await user.type(screen.getByRole('textbox'), 'unsaved');
    await user.click(screen.getByRole('button', { name: action }));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    expect(screen.getByTestId('location')).toHaveTextContent('/configuration#second');
    expect(screen.getByRole('textbox')).toHaveValue('unsaved');
  },
);

it('protects browser unload only while edits are unsaved and allows navigation after saving', async () => {
  const user = userEvent.setup();
  render(
    <TestRouter>
      <Editor />
      <Navigation />
    </TestRouter>,
  );
  await user.type(screen.getByRole('textbox'), 'unsaved');
  const unload = new Event('beforeunload', { cancelable: true });
  window.dispatchEvent(unload);
  expect(unload.defaultPrevented).toBe(true);
  await user.click(screen.getByRole('button', { name: 'Save Draft' }));
  const savedUnload = new Event('beforeunload', { cancelable: true });
  window.dispatchEvent(savedUnload);
  expect(savedUnload.defaultPrevented).toBe(false);
  await user.click(screen.getByRole('button', { name: 'Hash tab' }));
  expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  expect(screen.getByTestId('location')).toHaveTextContent('/#second');
});

it('uses one confirmation for multiple drafts and prevents discarding while a save is pending', async () => {
  const user = userEvent.setup();
  render(
    <TestRouter>
      <Editor />
      <Editor name='Nested draft' />
      <Navigation />
    </TestRouter>,
  );
  await user.type(screen.getByRole('textbox', { name: 'Draft' }), 'outer');
  await user.type(screen.getByRole('textbox', { name: 'Nested draft' }), 'inner');
  await user.click(screen.getByRole('button', { name: 'Saving Draft' }));
  await user.click(screen.getByRole('button', { name: 'Other page' }));
  expect(screen.getAllByRole('alertdialog')).toHaveLength(1);
  expect(screen.getByRole('button', { name: 'Discard changes' })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Keep editing' }));
  expect(screen.getByRole('textbox', { name: 'Nested draft' })).toHaveValue('inner');
});
