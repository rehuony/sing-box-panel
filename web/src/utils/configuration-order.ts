import { isLosslessNumber } from 'lossless-json';

import { compareText } from './compare-text';
import order from '../../../api/configuration-order.json';

export const configurationFieldOrder = order.fields;

export function orderConfigurationValue(value: unknown, root = false): unknown {
  if (Array.isArray(value)) return value.map(item => orderConfigurationValue(item));
  if (value === null || typeof value !== 'object' || isLosslessNumber(value)) return value;
  const keys = root ? order.root : order.fields;
  const rank = (key: string) => keys.includes(key) ? keys.indexOf(key) : keys.length;
  return Object.fromEntries(Object.entries(value)
    .sort(([left], [right]) => rank(left) - rank(right) || compareText(left, right))
    .map(([key, child]) => [key, orderConfigurationValue(child)]));
}
