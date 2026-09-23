import { expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';

import '@/i18n';
import { toast } from '@/components/ui/toast-manager';
import { ConfigurationJsonPreview } from '@/pages/configuration-page/configuration-json-preview';

it('keeps JSON available and reports clipboard failures', async () => {
  const user = userEvent.setup();
  vi.spyOn(navigator.clipboard, 'writeText').mockRejectedValue(new Error('Permission denied'));
  const notification = vi.spyOn(toast, 'add');
  const value = '{\n  "tag": "example"\n}';
  render(<ConfigurationJsonPreview value={value} />);
  await user.click(screen.getByRole('button', { name: 'Copy JSON' }));
  expect(screen.getByLabelText('JSON')).toHaveTextContent('"tag": "example"');
  expect(notification).toHaveBeenCalledWith({ title: 'Could not copy JSON. Select the text and copy it manually.', type: 'error' });
});
