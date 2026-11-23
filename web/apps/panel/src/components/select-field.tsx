import type { ComponentProps } from 'react';

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@shared/components/ui/select';

type SelectFieldProps<Value extends string | number> = Pick<
  ComponentProps<typeof SelectTrigger>,
  'id' | 'className' | 'disabled' | 'variant' | 'aria-label' | 'aria-labelledby' | 'aria-describedby' | 'aria-invalid'
> & {
  value: Value;
  items: { value: Value; label: string; disabled?: boolean }[];
  onValueChange: (value: Value) => void;
  /** In a bar, marks a choice that filters what the view shows. */
  active?: boolean;
};

export function SelectField<Value extends string | number>({
  value,
  items,
  onValueChange,
  disabled,
  active,
  ...props
}: SelectFieldProps<Value>) {
  return (
    <Select<Value>
      disabled={disabled}
      items={items}
      value={value}
      onValueChange={(next) => {
        if (next !== null) onValueChange(next);
      }}
    >
      <SelectTrigger data-active={active} {...props}><SelectValue /></SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value} disabled={item.disabled}>{item.label}</SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}
