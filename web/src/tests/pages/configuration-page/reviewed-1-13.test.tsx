import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import { resolve } from 'node:path';
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { fireEvent, render, screen, within } from '@testing-library/react';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';
import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

import '@/i18n';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { resolveReviewedSchema } from '@/schemas/resolve-reviewed-schema';
import { representativeSchemaVersions } from '@/tests/schemas/schema-fixtures';
import { SchemaSectionForm } from '@/pages/configuration-page/schema-section-form';
import { mergeSchemaKnownData, projectSchemaKnownData } from '@/pages/configuration-page/schema-ui';
import { ConfigurationSectionEditor } from '@/pages/configuration-page/configuration-section-editor';
import { encodeCanonicalDraft, parseCanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

const fixtures = [
  'dns-actions.json', 'dns-legacy.json', 'tls-transport.json', 'endpoints-services.json',
  'null-sections.json', 'log-defaults.json', 'log-null-fields.json',
].map((name) => ({
  name,
  text: readFileSync(resolve(process.cwd(), `../internal/singbox/testdata/configuration-1.13/${name}`), 'utf8'),
}));

function InboundHarness({ resolution }: { resolution: ReviewedSchemaResolution }) {
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

function LogHarness({ resolution, text }: { resolution: ReviewedSchemaResolution; text: string }) {
  const [draft, setDraft] = useState<CanonicalDraft>(() => parseCanonicalDraft(text));
  return (
    <>
      <SchemaSectionForm basePointer='/log' data={draft.log} onChange={setDraft}
        resolution={resolution} schema={resolution.schema.properties?.log as RJSFSchema} />
      <output aria-label='Reviewed draft'>{encodeCanonicalDraft(draft)}</output>
    </>
  );
}

function CollectionHarness({ resolution, text }: { resolution: ReviewedSchemaResolution; text: string }) {
  const [draft, setDraft] = useState<CanonicalDraft>(() => parseCanonicalDraft(text));
  return (
    <>
      <ConfigurationSectionEditor disabled={false} draft={draft} name='outbounds' onChange={setDraft}
        resolution={resolution} schema={resolution.schema.properties?.outbounds as RJSFSchema} />
      <output aria-label='Reviewed draft'>{encodeCanonicalDraft(draft)}</output>
    </>
  );
}

describe.each(representativeSchemaVersions('reviewed-1.13'))('reviewed 1.13 editor %s', (version) => {
  it('validates every section and preserves DNS, TLS, rules and protocols through form projection', async () => {
    const loaded = await reviewedSchemaManifest[version].load();
    const resolution = await resolveReviewedSchema({
      exact_version: version, schema_sha256: loaded.schemaSHA256, schema: loaded.schema,
    }, version);
    const validate = Object.values(loaded.validateFns)[0];
    for (const fixture of fixtures) {
      const draft = parseCanonicalDraft(fixture.text);
      expect(validate(JSON.parse(fixture.text)), fixture.name).toBe(true);
      const projected = projectSchemaKnownData(resolution.schema, resolution.schema, draft);
      const merged = mergeSchemaKnownData(
        resolution.schema, resolution.schema, draft, projected, structuredClone(projected),
      );
      expect(JSON.parse(encodeCanonicalDraft(merged as CanonicalDraft)), fixture.name)
        .toEqual(JSON.parse(fixture.text));
    }
    for (const value of [
      { outbounds: {} },
      { log: [] },
      { log: { disabled: 'false' } },
      { log: { level: 'verbose' } },
      { log: { colour: true } },
      { certificate: { providers: [{ type: 'acme', tag: 'cert' }] } },
      { inbounds: [{ type: 'snell', listen_port: 2080, psk: 'secret' }] },
      { endpoints: [{ type: 'openvpn', tag: 'vpn' }] },
      { dns: { servers: [{ type: 'mdns' }] } },
    ]) expect(validate(value)).toBe(false);
  }, 30_000);

  it('edits a real TLS inbound without losing inline ACME, transport or other sections', async () => {
    const loaded = await reviewedSchemaManifest[version].load();
    const resolution = await resolveReviewedSchema({
      exact_version: version, schema_sha256: loaded.schemaSHA256, schema: loaded.schema,
    }, version);
    render(<InboundHarness resolution={resolution} />);
    const port = screen.getByLabelText('Listen port');
    fireEvent.change(port, { target: { value: '8444' } });
    const expected = JSON.parse(fixtures[2].text);
    expected.inbounds[0].listen_port = 8444;
    expect(JSON.parse(screen.getByLabelText('Reviewed draft').textContent ?? '{}')).toEqual(expected);
    expect(screen.queryByText('Certificate provider')).not.toBeInTheDocument();
  }, 30_000);

  it.each(['null-sections.json', 'log-defaults.json', 'log-null-fields.json'])(
    'edits logging in %s without normalizing untouched nulls or defaults', async (name) => {
      const loaded = await reviewedSchemaManifest[version].load();
      const resolution = await resolveReviewedSchema({
        exact_version: version, schema_sha256: loaded.schemaSHA256, schema: loaded.schema,
      }, version);
      const fixture = fixtures.find((candidate) => candidate.name === name)!;
      render(<LogHarness resolution={resolution} text={fixture.text} />);
      const expected = JSON.parse(fixture.text);
      expect(JSON.parse(screen.getByLabelText('Reviewed draft').textContent ?? '{}')).toEqual(expected);
      expect(screen.getByText('Include timestamp')).toBeVisible();
      fireEvent.click(screen.getByRole('switch', { name: 'Include timestamp' }));
      expected.log = { ...expected.log, timestamp: true };
      expect(JSON.parse(screen.getByLabelText('Reviewed draft').textContent ?? '{}')).toEqual(expected);
      expect(Object.values(loaded.validateFns)[0](expected)).toBe(true);
    },
  );

  it('adds to a null collection only after confirmation and preserves other null sections', async () => {
    const user = userEvent.setup();
    const loaded = await reviewedSchemaManifest[version].load();
    const resolution = await resolveReviewedSchema({
      exact_version: version, schema_sha256: loaded.schemaSHA256, schema: loaded.schema,
    }, version);
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
    expect(Object.values(loaded.validateFns)[0](expected)).toBe(true);
  });
});
