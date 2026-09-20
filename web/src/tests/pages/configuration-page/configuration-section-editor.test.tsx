import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import { expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { customizeValidator } from '@rjsf/validator-ajv8';
import { fireEvent, render, screen } from '@testing-library/react';

import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

import '@/i18n';
import { encodeCanonicalValue } from '@/pages/configuration-page/use-canonical-configuration';
import { ConfigurationSectionEditor } from '@/pages/configuration-page/configuration-section-editor';

const dns: RJSFSchema = {
  type: 'object',
  properties: {
    servers: { type: 'array', items: { type: 'object', properties: { tag: { type: 'string' } } } },
    rules: { type: 'array', items: { type: 'object', properties: { action: { type: 'string' } } } },
    final: { type: 'string' },
  },
};
const root: RJSFSchema = { type: 'object', properties: { dns } };

function Harness() {
  const [draft, setDraft] = useState<CanonicalDraft>({
    dns: { servers: [{ tag: 'remote', future: 42 }], rules: [{ action: 'route' }], final: 'remote', unknown: true },
  });
  return (
    <>
      <ConfigurationSectionEditor disabled={false} draft={draft} name='dns' onChange={setDraft}
        resolution={{ schema: root, createValidator: () => customizeValidator() }} schema={dns} />
      <output aria-label='Draft'>{encodeCanonicalValue(draft)}</output>
    </>
  );
}

it('separates lists from settings without erasing fields in the other tabs', async () => {
  const user = userEvent.setup();
  render(<Harness />);
  expect(screen.getByRole('button', { name: 'remote' })).toBeInTheDocument();
  expect(screen.queryByRole('textbox', { name: 'Default destination' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('tab', { name: 'Resolution & cache' }));
  fireEvent.change(screen.getByRole('textbox', { name: 'Default destination' }), { target: { value: 'local' } });
  await user.click(screen.getByRole('tab', { name: 'Servers' }));
  expect(screen.getByRole('button', { name: 'remote' })).toBeInTheDocument();
  expect(JSON.parse(screen.getByLabelText('Draft').textContent ?? '{}')).toEqual({
    dns: { servers: [{ tag: 'remote', future: 42 }], rules: [{ action: 'route' }], final: 'local', unknown: true },
  });
});
