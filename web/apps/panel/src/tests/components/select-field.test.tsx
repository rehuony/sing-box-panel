import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';

import { SelectField } from '@/components/select-field';

describe('selectField', () => {
  it('keeps numeric values and skips disabled options during keyboard selection', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    function Example() {
      const [value, setValue] = useState(0);
      return (
        <SelectField
          aria-label='Interval'
          value={value}
          items={[
            { value: 0, label: 'Off' },
            { value: 10, label: 'Unavailable', disabled: true },
            { value: 20, label: 'Twenty minutes' },
          ]}
          onValueChange={(next) => {
            setValue(next);
            onChange(next);
          }}
        />
      );
    }
    render(<Example />);
    const trigger = screen.getByRole('combobox', { name: 'Interval' });
    await user.click(trigger);
    const unavailable = await screen.findByRole('option', { name: 'Unavailable' });
    expect(unavailable).toHaveAttribute('aria-disabled', 'true');
    await user.click(unavailable);
    expect(onChange).not.toHaveBeenCalled();
    await user.keyboard('{End}{Enter}');
    expect(trigger).toHaveTextContent('Twenty minutes');
    expect(trigger).toHaveFocus();
    expect(onChange).toHaveBeenCalledExactlyOnceWith(20);
  });

  it('supports an empty option and passes validation and disabled state to the trigger', async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();
    const props = {
      'aria-label': 'Source format',
      'aria-invalid': true,
      'aria-describedby': 'format-error',
      'value': 'json',
      'items': [{ value: '', label: 'Choose format' }, { value: 'json', label: 'JSON' }],
      onValueChange,
    } as const;
    const { rerender } = render(<SelectField {...props} items={[...props.items]} />);
    const trigger = screen.getByRole('combobox', { name: 'Source format' });
    expect(trigger).toHaveAttribute('aria-invalid', 'true');
    expect(trigger).toHaveAttribute('aria-describedby', 'format-error');
    await user.click(trigger);
    await user.click(await screen.findByRole('option', { name: 'Choose format' }));
    expect(onValueChange).toHaveBeenCalledExactlyOnceWith('');
    rerender(<SelectField {...props} items={[...props.items]} disabled />);
    expect(trigger).toBeDisabled();
    await user.click(trigger);
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
  });
});
