import type { ComponentProps } from 'react';

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

type SelectFieldProps<Value extends string | number> = Pick<
  ComponentProps<typeof SelectTrigger>,
  'id' | 'className' | 'disabled' | 'aria-label' | 'aria-labelledby' | 'aria-describedby'
> & {
  value: Value;
  items: { value: Value; label: string }[];
  onValueChange: (value: Value) => void;
};

export function SelectField<Value extends string | number>({
  value,
  items,
  onValueChange,
  disabled,
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
      <SelectTrigger {...props}><SelectValue /></SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}
