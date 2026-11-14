import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';

import '@/i18n';
import { TagInput } from '@/components/tag-input';

function Harness({ onReject }: { onReject: (reason: string) => void }) {
  const [tags, setTags] = useState<string[]>(['jp']);
  return (
    <>
      <TagInput id='tags' onChange={setTags} onReject={onReject} value={tags} />
      <output aria-label='Tags'>{tags.join(' ')}</output>
    </>
  );
}

describe('tag input', () => {
  it('adds tags in lower case, keeps them sorted and distinct, and refuses what a tag cannot be', async () => {
    const user = userEvent.setup();
    const onReject = vi.fn();
    render(<Harness onReject={onReject} />);
    const input = screen.getByRole('textbox');
    await user.type(input, 'HK,premium jp ');
    expect(screen.getByLabelText('Tags')).toHaveTextContent(/^hk jp premium$/);
    await user.type(input, 'bad!{Enter}');
    expect(onReject).toHaveBeenLastCalledWith('invalid');
    expect(screen.getByLabelText('Tags')).toHaveTextContent(/^hk jp premium$/);
    await user.clear(input);
    await user.type(input, '{Backspace}');
    expect(screen.getByLabelText('Tags')).toHaveTextContent(/^hk jp$/);
    await user.click(screen.getByRole('button', { name: 'Remove tag hk' }));
    expect(screen.getByLabelText('Tags')).toHaveTextContent(/^jp$/);
  });
});
