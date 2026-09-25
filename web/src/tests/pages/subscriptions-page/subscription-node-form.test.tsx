import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';
import { beforeAll, describe, expect, it } from 'vitest';
import { createPrecompiledValidator } from '@rjsf/validator-ajv8';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';

import '@/i18n';
import { reviewedSchemaManifest } from '@/schemas/generated';
import { SubscriptionNodeForm } from '@/pages/subscriptions-page/subscription-node-form';

let resolution: ReviewedSchemaResolution;
beforeAll(async () => {
  const reviewed = await reviewedSchemaManifest['1.14.1'].load();
  resolution = {
    schema: reviewed.schema,
    createValidator: root => createPrecompiledValidator(reviewed.validateFns as never, root),
  };
});

function Harness({ initial }: { initial: Record<string, unknown> }) {
  const [node, setNode] = useState({
    tag: 'node', server: 'example.com', server_port: 443, future: { keep: true }, ...initial,
  });
  const schema = (resolution.schema.properties?.outbounds as RJSFSchema).items as RJSFSchema;
  return (
    <>
      <SubscriptionNodeForm data={node} candidates={[]} disabled={false} schema={schema}
        resolution={resolution} onChange={raw => setNode(JSON.parse(raw))} />
      <output aria-label='Node draft'>{JSON.stringify(node)}</output>
    </>
  );
}

function draft() {
  return JSON.parse(screen.getByLabelText('Node draft').textContent!);
}

describe('subscription node credentials', () => {
  it('uses the current method while preserving the password until the dice is clicked', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ type: 'shadowsocks', method: '2022-blake3-aes-128-gcm', password: 'keep' }} />);
    for (const [method, length] of [
      ['2022-blake3-aes-256-gcm', 32],
      ['2022-blake3-aes-128-gcm', 16],
      ['aes-128-gcm', 0],
    ] as const) {
      const before = draft();
      screen.getByRole('combobox', { name: 'Request method' }).focus();
      await user.keyboard('[ArrowDown]');
      await user.click(await screen.findByRole('option', { name: method }));
      expect(draft().password).toBe(before.password);
      await user.click(screen.getByRole('button', { name: 'Generate random Password' }));
      if (length) expect(atob(draft().password)).toHaveLength(length);
      else expect(draft().password).toMatch(/^[\w-]{24}$/);
      expect(draft()).toEqual({ ...before, method, password: draft().password });
    }
    const previous = draft().password;
    screen.getByRole('combobox', { name: 'Request method' }).focus();
    await user.keyboard('[ArrowDown]');
    await user.click(await screen.findByRole('option', { name: 'none' }));
    expect(screen.queryByRole('button', { name: 'Generate random Password' })).not.toBeInTheDocument();
    expect(draft().password).toBe(previous);
  });

  it.each([undefined, 'none', 'future-method'])('omits generation for Shadowsocks method %s', method => {
    render(<Harness initial={{ type: 'shadowsocks', method, password: 'keep' }} />);
    expect(screen.queryByRole('button', { name: 'Generate random Password' })).not.toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Password' })).toHaveValue('keep');
  });

  it('refreshes credential rules when the protocol changes', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ type: 'shadowsocks', method: '2022-blake3-aes-128-gcm', password: 'keep' }} />);
    screen.getByRole('combobox', { name: 'Protocol' }).focus();
    await user.keyboard('[ArrowDown]');
    await user.click(await screen.findByRole('option', { name: 'trojan' }));
    expect(draft().password).toBe('keep');
    await user.click(screen.getByRole('button', { name: 'Generate random Password' }));
    expect(draft().password).toMatch(/^[\w-]{24}$/);
    const previous = draft().password;
    screen.getByRole('combobox', { name: 'Protocol' }).focus();
    await user.keyboard('[ArrowDown]');
    await user.click(await screen.findByRole('option', { name: 'shadowsocks' }));
    expect(screen.queryByRole('button', { name: 'Generate random Password' })).not.toBeInTheDocument();
    expect(draft()).toMatchObject({ type: 'shadowsocks', password: previous, future: { keep: true } });
    screen.getByRole('combobox', { name: 'Request method' }).focus();
    await user.keyboard('[ArrowDown]');
    await user.click(await screen.findByRole('option', { name: '2022-blake3-aes-128-gcm' }));
    await user.click(screen.getByRole('button', { name: 'Generate random Password' }));
    expect(atob(draft().password)).toHaveLength(16);
  });

  it('retains protocol context for Snell PSKs', async () => {
    const user = userEvent.setup();
    render(<Harness initial={{ type: 'snell', version: 6, psk: 'keep' }} />);
    const before = draft();
    await user.click(screen.getByRole('button', { name: 'Generate random psk' }));
    expect(draft().psk).toMatch(/^[\w-]{24}$/);
    expect(draft()).toEqual({ ...before, psk: draft().psk });
  });
});
