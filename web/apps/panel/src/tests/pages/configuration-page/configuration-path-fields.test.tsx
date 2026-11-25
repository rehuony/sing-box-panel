import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import { ApiClientProvider } from '@/api/api-client-context';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { createFakeFilesystemApi } from '@/tests/api/fake-filesystem';
import { configurationSchemas } from '@/tests/fixtures/configuration-schemas';
import { SchemaSectionForm } from '@/pages/configuration-page/schema-section-form';
import { schemaProperties, selfContainedSchema } from '@/pages/configuration-page/schema-ui';
import { readConfigurationPathMode } from '@/pages/configuration-page/configuration-path-fields';
import { encodeCanonicalDraft, parseCanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

const schemas = configurationSchemas;

describe('configuration path presentation', () => {
  it.each(Object.entries(schemas))('covers filesystem properties without changing the authoritative schema: %s', (_, root) => {
    const original = JSON.stringify(root);
    const presentation = selfContainedSchema(root, root);
    const missing: string[] = [];
    const unexpected: string[] = [];
    const modes = new Set<string>();
    function visit(value: unknown, location: string) {
      if (Array.isArray(value)) {
        value.forEach((child, index) => visit(child, `${location}/${index}`));
      } else if (value && typeof value === 'object') {
        const schema = value as RJSFSchema;
        for (const [key, child] of Object.entries(schema.properties ?? {})) {
          if (typeof child !== 'object') continue;
          const mode = readConfigurationPathMode(child);
          // These names unambiguously refer to real filesystem resources in
          // the test schemas. Ambiguous `path` is checked separately below.
          if ((key.endsWith('_path') && key !== 'tcp_multi_path') || key.endsWith('_directory')
            || ['external_ui', 'directory', 'mesh_psk_file', 'dhcp_lease_files', 'pid_file'].includes(key)) {
            if (!mode) missing.push(`${location}/${key}`);
          }
          if ((key.endsWith('_url') || key === 'process_path_regex' || key === 'netns' || key === 'tcp_multi_path') && mode) {
            unexpected.push(`${location}/${key}`);
          }
          if (mode) modes.add(mode);
        }
        for (const [key, child] of Object.entries(schema)) {
          if (!key.startsWith('x-panel')) visit(child, `${location}/${key}`);
        }
      }
    }
    visit(presentation, '');
    expect(missing).toEqual([]);
    expect(unexpected).toEqual([]);
    expect([...modes].sort()).toEqual(['directory', 'file', 'output-file', 'socket']);
    expect(JSON.stringify(root)).toBe(original);

    const definitions = presentation.$defs as Record<string, RJSFSchema>;
    const dns = definitions.DNSServer ?? definitions.DNSServerOptions;
    for (const type of ['https', 'h3']) {
      expect(readConfigurationPathMode(schemaProperties(dns, presentation, { type }).path)).toBeUndefined();
    }
    expect(readConfigurationPathMode(schemaProperties(dns, presentation, { type: 'hosts' }).path)).toBe('file');
    expect(readConfigurationPathMode(schemaProperties(definitions.RuleSet, presentation, { type: 'local' }).path)).toBe('file');
    expect(readConfigurationPathMode(schemaProperties(definitions.Outbound, presentation, { type: 'http' }).path)).toBeUndefined();
    expect(readConfigurationPathMode(schemaProperties(definitions.CacheFileOptions, presentation).path)).toBe('output-file');
    expect(readConfigurationPathMode(schemaProperties(definitions.Service, presentation, { type: 'derp' }).config_path)).toBe('output-file');
    const transport = definitions.V2RayTransport ?? definitions.V2RayTransportOptions;
    for (const type of ['http', 'ws', 'httpupgrade']) {
      expect(readConfigurationPathMode(schemaProperties(transport, presentation, { type }).path)).toBeUndefined();
    }
    if (definitions.NetworkNamespace) {
      expect(readConfigurationPathMode(schemaProperties(definitions.NetworkNamespace, presentation, { type: 'default' }).path)).toBe('file');
      expect(readConfigurationPathMode(schemaProperties(definitions.Service, presentation, { type: 'api' }).dashboard)).toBe('directory');
    }
  });

  it('uses the picker for array elements and updates only the selected field in the lossless draft', async () => {
    const schema: RJSFSchema = { type: 'object', properties: {
      certificate_path: { anyOf: [{ type: 'string' }, { type: 'array', items: { type: 'string' } }] },
      tag: { type: 'string' },
    } };
    function Harness() {
      const [draft, setDraft] = useState(() => parseCanonicalDraft('{"certificate":{"certificate_path":["/etc/sing-box/certificate.pem"],"tag":"keep","future":900719925474099312345}}'));
      return (
        <>
          <SchemaSectionForm schema={schema} basePointer='/certificate' data={draft.certificate} onChange={setDraft}
            resolution={{ schema }} />
          <output data-testid='draft'>{encodeCanonicalDraft(draft)}</output>
        </>
      );
    }
    const api = createMockApiClient(createFakeFilesystemApi());
    render(<ApiClientProvider client={api}><Harness /></ApiClientProvider>);
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: /Browse server path:/ }));
    const dialog = within(screen.getByRole('dialog'));
    await user.click(await dialog.findByRole('button', { name: 'private.key' }));
    await user.click(dialog.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(screen.getByTestId('draft').textContent).toContain('/etc/sing-box/private.key'));
    expect(screen.getByTestId('draft').textContent).toContain('900719925474099312345');
    expect(screen.getByTestId('draft').textContent).toContain('"tag":"keep"');
    expect(screen.getByTestId('draft').textContent).not.toContain('x-panel');
  });
});
