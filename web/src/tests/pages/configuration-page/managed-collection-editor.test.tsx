import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import { ManagedCollectionEditor } from '@/pages/configuration-page/managed-collection-editor';

describe('managedCollectionEditor', () => {
  it('offers only inbound types supported by the reviewed core contracts', () => {
    render(
      <ManagedCollectionEditor
        collection='inbounds'
        draft={{
          schema_version: 2,
          configuration: {
            inbounds: [{ _panel: { id: 'mixed', enabled: true }, tag: 'mixed', type: 'mixed' }],
          },
        }}
        onChange={vi.fn()}
      />,
    );

    const type = screen.getByLabelText('Type') as HTMLSelectElement;
    const options = [...type.options].map(option => option.value);
    expect(options).toContain('anytls');
    expect(options).not.toContain('snell');
  });
});
