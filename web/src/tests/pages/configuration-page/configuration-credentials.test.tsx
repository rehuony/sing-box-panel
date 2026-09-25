import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import userEvent from '@testing-library/user-event';
import { createPrecompiledValidator } from '@rjsf/validator-ajv8';
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/react';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import '@/i18n';
import { toast } from '@/components/ui/toast-manager';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { SchemaSectionForm } from '@/pages/configuration-page/schema-section-form';
import { schemaProperties, selfContainedSchema, uiSchemaFromPanel } from '@/pages/configuration-page/schema-ui';
import { encodeCanonicalDraft, parseCanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';
import { configurationCredentialKind, credentialGenerator, generateCredential } from '@/pages/configuration-page/configuration-credentials';

const schemas = import.meta.glob<RJSFSchema>('../../../schemas/generated/schema-*.json', { eager: true, import: 'default' });
let resolution: ReviewedSchemaResolution;
beforeAll(async () => {
  const reviewed = await reviewedSchemaManifest['1.14.1'].load();
  resolution = {
    schema: reviewed.schema,
    createValidator: root => createPrecompiledValidator(reviewed.validateFns as never, root),
  };
});
afterEach(() => vi.restoreAllMocks());

function Harness({ initial, collection = 'inbounds', disabled = false, readonly = false }: {
  initial: Record<string, unknown>;
  collection?: 'inbounds' | 'outbounds';
  disabled?: boolean;
  readonly?: boolean;
}) {
  const [draft, setDraft] = useState(() => parseCanonicalDraft(
    `{"${collection}":[${JSON.stringify(initial).slice(0, -1)},"future":900719925474099312345}]}`,
  ));
  const data = (draft[collection] as unknown[])[0];
  const schema = (resolution.schema.properties?.[collection] as RJSFSchema).items as RJSFSchema;
  return (
    <>
      <SchemaSectionForm basePointer={`/${collection}/0`} data={data} onChange={setDraft}
        schema={schema} resolution={resolution} disabled={disabled}
        uiSchema={uiSchemaFromPanel(schema, readonly ? ['password'] : [], resolution.schema, data)} />
      <output aria-label='Credential draft'>{encodeCanonicalDraft(draft)}</output>
    </>
  );
}

describe('credential formats', () => {
  it.each(Object.entries(schemas))('annotates reviewed credential fields without mutating %s', (_, root) => {
    const before = JSON.stringify(root);
    const presentation = selfContainedSchema(root, root);
    const defs = presentation.$defs as Record<string, RJSFSchema>;
    const inbound = schemaProperties(defs.Inbound, presentation, { type: 'shadowsocks' });
    expect(credentialGenerator(inbound.password, '2022-blake3-aes-128-gcm')).toBe('base64-16');
    expect(credentialGenerator(schemaProperties(defs.ShadowsocksUser, presentation).password, '2022-blake3-aes-256-gcm')).toBe('base64-32');
    expect(credentialGenerator(schemaProperties(defs.ShadowsocksDestination, presentation).password, '2022-blake3-chacha20-poly1305')).toBe('base64-32');
    expect(credentialGenerator(schemaProperties(defs.TUICUser, presentation).uuid)).toBe('uuid');
    expect(credentialGenerator(schemaProperties(defs.WireGuardPeer, presentation).pre_shared_key)).toBe('base64-32');
    expect(credentialGenerator(schemaProperties(defs.OutboundRealityOptions, presentation).public_key)).toBeUndefined();
    expect(JSON.stringify(root)).toBe(before);
  });

  it.each([
    ['password', ['TrojanUser'], 'password'],
    ['Password', ['User'], 'password'],
    ['auth_str', ['HysteriaUser'], 'password'],
    ['auth', ['HysteriaUser'], 'base64'],
    ['obfs', ['inbounds', 'hysteria'], 'password'],
    ['psk', ['snell'], 'password'],
    ['userkey', ['SnellUser'], 'password'],
    ['secret', ['ClashAPIOptions'], 'password'],
    ['auth_key', ['tailscale'], undefined],
    ['private_key', ['InboundRealityOptions'], undefined],
    ['key', ['InboundTLSOptions'], undefined],
    ['mac_key', ['ACMEExternalAccountOptions'], undefined],
  ] as const)('classifies %s in %j', (key, context, expected) => {
    expect(configurationCredentialKind(key, context)).toBe(expected);
  });

  it.each(['base64-16', 'base64-32'] as const)('generates correctly sized raw keys for %s', generator => {
    const value = generateCredential(generator);
    expect(atob(value)).toHaveLength(generator === 'base64-16' ? 16 : 32);
    expect(btoa(atob(value))).toBe(value);
    expect(generateCredential(generator)).not.toBe(value);
  });

  it('generates 24 ASCII password characters and UUID v4 without requiring a secure context', () => {
    const random = vi.spyOn(crypto, 'getRandomValues');
    expect(generateCredential('password')).toMatch(/^[\w-]{24}$/);
    expect(generateCredential('uuid')).toMatch(/^[\da-f]{8}-[\da-f]{4}-4[\da-f]{3}-[89ab][\da-f]{3}-[\da-f]{12}$/);
    expect(random).toHaveBeenCalledTimes(2);
  });
});

describe('credential inputs', () => {
  it.each([
    ['2022-blake3-aes-128-gcm', 16],
    ['2022-blake3-aes-256-gcm', 32],
    ['2022-blake3-chacha20-poly1305', 32],
  ] as const)('generates a visible %s password in the canonical draft', async (method, length) => {
    const user = userEvent.setup();
    render(<Harness initial={{ type: 'shadowsocks', tag: 'ss', method, password: 'old' }} />);
    const input = screen.getByRole('textbox', { name: 'Password' });
    const button = screen.getByRole('button', { name: 'Generate random Password' });
    expect(input).toHaveAttribute('type', 'text');
    await user.click(button);
    const value = (input as HTMLInputElement).value;
    expect(atob(value)).toHaveLength(length);
    expect(screen.getByLabelText('Credential draft')).toHaveTextContent(JSON.stringify(value));
    expect(screen.getByLabelText('Credential draft')).toHaveTextContent('900719925474099312345');
  });

  it('uses the current method without automatically replacing existing passwords', async () => {
    const user = userEvent.setup();
    render(<Harness collection='outbounds' initial={{ type: 'shadowsocks', method: 'aes-128-gcm', password: 'keep' }} />);
    const input = screen.getByRole('textbox', { name: 'Password' });
    await user.click(screen.getByRole('button', { name: 'Generate random Password' }));
    expect((input as HTMLInputElement).value).toMatch(/^[\w-]{24}$/);
    const original = (input as HTMLInputElement).value;
    await user.click(screen.getByRole('combobox', { name: 'Request method' }));
    await user.click(await screen.findByRole('option', { name: '2022-blake3-aes-256-gcm' }));
    expect(input).toHaveValue(original);
    await user.click(screen.getByRole('button', { name: 'Generate random Password' }));
    expect(atob((input as HTMLInputElement).value)).toHaveLength(32);
    fireEvent.change(input, { target: { value: 'manual-password' } });
    expect(input).toHaveValue('manual-password');
  });

  it.each([undefined, 'none'])('omits Shadowsocks generation for method %s', method => {
    render(<Harness initial={{ type: 'shadowsocks', method, password: 'keep' }} />);
    expect(screen.getByRole('textbox', { name: 'Password' })).toHaveValue('keep');
    expect(screen.queryByRole('button', { name: 'Generate random Password' })).not.toBeInTheDocument();
  });

  it.each(['users', 'destinations'])('inherits key length in a nested %s dialog and stages changes until confirmed', async collection => {
    const user = userEvent.setup();
    render(<Harness initial={{ type: 'shadowsocks', method: '2022-blake3-aes-128-gcm', password: 'server-key', [collection]: [{ name: 'alice', password: 'original' }] }} />);
    const original = screen.getByLabelText('Credential draft').textContent;
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    let dialog = within(screen.getByRole('dialog'));
    await user.click(dialog.getByRole('button', { name: 'Generate random Password' }));
    expect(atob((dialog.getByRole('textbox', { name: 'Password' }) as HTMLInputElement).value)).toHaveLength(16);
    expect(screen.getByLabelText('Credential draft').textContent).toBe(original);
    await user.click(dialog.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByLabelText('Credential draft').textContent).toBe(original);
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    dialog = within(screen.getByRole('dialog'));
    expect(dialog.getByRole('textbox', { name: 'Password' })).toHaveValue('original');
    await user.click(dialog.getByRole('button', { name: 'Generate random Password' }));
    const value = (dialog.getByRole('textbox', { name: 'Password' }) as HTMLInputElement).value;
    await user.click(dialog.getByRole('button', { name: 'Save changes' }));
    expect(screen.getByLabelText('Credential draft')).toHaveTextContent(JSON.stringify(value));
    expect(screen.getByLabelText('Credential draft')).toHaveTextContent('server-key');
  });

  it('generates independent TUIC password and UUID credentials', async () => {
    const user = userEvent.setup();
    render(<Harness collection='outbounds' initial={{ type: 'tuic', password: 'keep', uuid: 'old-uuid' }} />);
    await user.click(screen.getByRole('button', { name: /Generate random uuid/i }));
    expect((screen.getByRole('textbox', { name: /^uuid$/i }) as HTMLInputElement).value).toMatch(/^[\da-f-]{36}$/);
    expect(screen.getByRole('textbox', { name: 'Password' })).toHaveValue('keep');
    await user.click(screen.getByRole('button', { name: 'Generate random Password' }));
    expect((screen.getByRole('textbox', { name: 'Password' }) as HTMLInputElement).value).toMatch(/^[\w-]{24}$/);
  });

  it('generates a Snell v6 PSK within its byte limits', async () => {
    const user = userEvent.setup();
    render(<Harness collection='outbounds' initial={{ type: 'snell', version: 6, psk: 'keep' }} />);
    await user.click(screen.getByRole('button', { name: 'Generate random psk' }));
    const value = (screen.getByRole('textbox', { name: 'psk' }) as HTMLInputElement).value;
    expect(new TextEncoder().encode(value).length).toBeGreaterThanOrEqual(12);
    expect(new TextEncoder().encode(value).length).toBeLessThanOrEqual(255);
  });

  it.each([{ disabled: true }, { readonly: true }])('respects non-editable state %j', state => {
    render(<Harness collection='outbounds' initial={{ type: 'trojan', password: 'keep' }} {...state} />);
    expect(screen.getByRole('button', { name: 'Generate random Password' })).toBeDisabled();
    expect(screen.getByRole('textbox', { name: 'Password' })).toHaveValue('keep');
  });

  it('retains the existing value if secure randomness fails', async () => {
    const user = userEvent.setup();
    render(<Harness collection='outbounds' initial={{ type: 'trojan', password: 'keep' }} />);
    const add = vi.spyOn(toast, 'add');
    vi.spyOn(crypto, 'getRandomValues').mockImplementation(() => {
      throw new Error('unavailable');
    });
    await user.click(screen.getByRole('button', { name: 'Generate random Password' }));
    expect(screen.getByRole('textbox', { name: 'Password' })).toHaveValue('keep');
    expect(add).toHaveBeenCalledWith({ title: 'Could not generate a credential. Try again.', type: 'error' });
  });
});
