import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import { resolve } from 'node:path';
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { fireEvent, render, screen, within } from '@testing-library/react';

import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';
import type { ConfigurationSchemaResolution } from '@/pages/configuration-page/use-configuration-schema';

import '@/i18n';
import { configurationSchemas } from '@/tests/fixtures/configuration-schemas';
import { SchemaSectionForm } from '@/pages/configuration-page/schema-section-form';
import { ConfigurationSectionEditor } from '@/pages/configuration-page/configuration-section-editor';
import { encodeCanonicalDraft, parseCanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

const fixtures = [
  'dns-actions.json', 'dns-legacy.json', 'tls-transport.json', 'endpoints-services.json',
  'null-sections.json', 'log-defaults.json', 'log-null-fields.json',
].map((name) => ({
  name,
  text: readFileSync(resolve(import.meta.dirname, `../../../../../../../internal/sing-box-panel/schema/testdata/configuration-1.13/${name}`), 'utf8'),
}));

function InboundHarness({ resolution }: { resolution: ConfigurationSchemaResolution }) {
  const [draft, setDraft] = useState<CanonicalDraft>(() => parseCanonicalDraft(fixtures[2].text));
  const inbounds = draft.inbounds as unknown[];
  const section = resolution.schema.properties?.inbounds as RJSFSchema;
  return (
    <>
      <SchemaSectionForm basePointer='/inbounds/0' data={inbounds[0]} onChange={setDraft}
        resolution={resolution} schema={section.items as RJSFSchema} />
      <output aria-label='Reviewed draft'>{encodeCanonicalDraft(draft)}</output>
    </>
  );
}

function LogHarness({ resolution, text }: { resolution: ConfigurationSchemaResolution; text: string }) {
  const [draft, setDraft] = useState<CanonicalDraft>(() => parseCanonicalDraft(text));
  return (
    <>
      <SchemaSectionForm basePointer='/log' data={draft.log} onChange={setDraft}
        resolution={resolution} schema={resolution.schema.properties?.log as RJSFSchema} />
      <output aria-label='Reviewed draft'>{encodeCanonicalDraft(draft)}</output>
    </>
  );
}

function CollectionHarness({ resolution, text }: { resolution: ConfigurationSchemaResolution; text: string }) {
  const [draft, setDraft] = useState<CanonicalDraft>(() => parseCanonicalDraft(text));
  return (
    <>
      <ConfigurationSectionEditor disabled={false} draft={draft} name='outbounds' onChange={setDraft}
        resolution={resolution} schema={resolution.schema.properties?.outbounds as RJSFSchema} />
      <output aria-label='Reviewed draft'>{encodeCanonicalDraft(draft)}</output>
    </>
  );
}

describe('reviewed 1.13 editor', () => {
  const resolution: ConfigurationSchemaResolution = { schema: configurationSchemas['1.13'] };

  it('edits a real TLS inbound without losing inline ACME, transport or other sections', () => {
    render(<InboundHarness resolution={resolution} />);
    const port = screen.getByLabelText('Listen port');
    fireEvent.change(port, { target: { value: '8444' } });
    const expected = JSON.parse(fixtures[2].text);
    expected.inbounds[0].listen_port = 8444;
    expect(JSON.parse(screen.getByLabelText('Reviewed draft').textContent ?? '{}')).toEqual(expected);
    expect(screen.queryByText('Certificate provider')).not.toBeInTheDocument();
  });

  it.each(['null-sections.json', 'log-defaults.json', 'log-null-fields.json'])(
    'edits logging in %s without normalizing untouched nulls or defaults', (name) => {
      const fixture = fixtures.find((candidate) => candidate.name === name)!;
      render(<LogHarness resolution={resolution} text={fixture.text} />);
      const expected = JSON.parse(fixture.text);
      expect(JSON.parse(screen.getByLabelText('Reviewed draft').textContent ?? '{}')).toEqual(expected);
      expect(screen.getByText('Include timestamp')).toBeVisible();
      fireEvent.click(screen.getByRole('switch', { name: 'Include timestamp' }));
      expected.log = { ...expected.log, timestamp: true };
      expect(JSON.parse(screen.getByLabelText('Reviewed draft').textContent ?? '{}')).toEqual(expected);
    },
  );

  it('adds to a null collection only after confirmation and preserves other null sections', async () => {
    const user = userEvent.setup();
    const fixture = fixtures.find((candidate) => candidate.name === 'null-sections.json')!;
    const expected = JSON.parse(fixture.text);
    render(<CollectionHarness resolution={resolution} text={fixture.text} />);
    expect(JSON.parse(screen.getByLabelText('Reviewed draft').textContent ?? '{}')).toEqual(expected);
    await user.click(screen.getByRole('button', { name: 'Add' }));
    let dialog = screen.getByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(JSON.parse(screen.getByLabelText('Reviewed draft').textContent ?? '{}')).toEqual(expected);
    await user.click(screen.getByRole('button', { name: 'Add' }));
    dialog = screen.getByRole('dialog');
    await user.click(within(dialog).getByRole('combobox', { name: 'Type' }));
    await user.click(await screen.findByRole('option', { name: /^direct$/i }));
    fireEvent.change(within(dialog).getByRole('textbox', { name: 'Tag' }), { target: { value: 'direct' } });
    await user.click(within(dialog).getByRole('button', { name: 'Add' }));
    expected.outbounds = [{ type: 'direct', tag: 'direct' }];
    expect(JSON.parse(screen.getByLabelText('Reviewed draft').textContent ?? '{}')).toEqual(expected);
  });
});
