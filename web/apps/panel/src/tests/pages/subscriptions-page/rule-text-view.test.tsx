import { expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { RuleTextView } from '@/pages/subscriptions-page/rule-text-view';

it('draws a screenful of a long text\'s line numbers, which follow its scroll with their marks', async () => {
  const text = Array.from({ length: 5000 }, (_, index) => `DOMAIN,host${index}.example`).join('\n');
  render(
    <RuleTextView
      describe={issue => `Line ${issue.line} cannot be read`}
      invalid
      label='Rules'
      lineCount={5000}
      marks={{
        changed: new Set(),
        errors: new Set([4000]),
        issues: new Map([[4000, { code: 'invalid_domain', line: 4000 }]]),
        notes: new Set(),
      }}
      onChange={() => undefined}
      ref={null}
      text={text}
    />,
  );
  const area = screen.getByRole('textbox', { name: 'Rules' });
  expect(screen.getByText('1')).toBeInTheDocument();
  expect(screen.queryByText('4000')).not.toBeInTheDocument();

  Object.defineProperty(area, 'scrollTop', { configurable: true, value: 3990 * 20 });
  fireEvent.scroll(area);
  expect(await screen.findByText('4000')).toHaveAttribute('title', 'Line 4000 cannot be read');
  expect(screen.queryByText('1')).not.toBeInTheDocument();
});
