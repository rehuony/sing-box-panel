import type { RJSFSchema } from '@rjsf/utils';

import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';
import { customizeValidator } from '@rjsf/validator-ajv8';

import type { ReviewedSchemaResolution } from '@/schemas/resolve-reviewed-schema';
import type { CanonicalDraft } from '@/pages/configuration-page/use-canonical-configuration';

import '@/i18n';
import { SchemaSectionForm } from '@/pages/configuration-page/schema-section-form';

const sectionSchema: RJSFSchema = {
  type: 'object',
  additionalProperties: false,
  properties: {
    users: {
      type: 'array',
      items: {
        type: 'object',
        additionalProperties: false,
        properties: {
          name: { type: 'string' },
          profile: {
            type: 'object',
            additionalProperties: false,
            properties: { known: { type: 'string' } },
          },
        },
      },
    },
  },
};

const rootSchema: RJSFSchema = {
  type: 'object',
  additionalProperties: false,
  properties: { section: sectionSchema },
};

const resolution: ReviewedSchemaResolution = {
  schema: rootSchema,
  createValidator: () => customizeValidator(),
};

function Harness() {
  const [draft, setDraft] = useState<CanonicalDraft>({
    section: {
      users: [
        {
          name: 'A',
          profile: { future: { owner: 'A' }, known: 'alpha' },
        },
        {
          name: 'B',
          profile: { future: { owner: 'B' }, known: 'beta' },
        },
      ],
    },
  });
  return (
    <>
      <SchemaSectionForm
        basePointer='/section'
        data={draft.section}
        onChange={(change) => setDraft((current) => change(current))}
        resolution={resolution}
        schema={sectionSchema}
      />
      <output aria-label='Canonical draft'>{JSON.stringify(draft)}</output>
    </>
  );
}

describe('schemaSectionForm', () => {
  it('keeps nested unknown fields attached when same-length array members are reordered', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    await user.click(screen.getAllByRole('button', { name: 'Move down' })[0]);

    const encoded = screen.getByLabelText('Canonical draft').textContent ?? '';
    const draft = JSON.parse(encoded) as CanonicalDraft;
    expect(draft.section).toEqual({
      users: [
        {
          name: 'B',
          profile: { future: { owner: 'B' }, known: 'beta' },
        },
        {
          name: 'A',
          profile: { future: { owner: 'A' }, known: 'alpha' },
        },
      ],
    });
  });
});
