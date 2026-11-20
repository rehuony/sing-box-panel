import { useTranslation } from 'react-i18next';

import type { TokenSplit } from '@/components/token-input';

import { TokenInput } from '@/components/token-input';

/** A node carries at most this many tags. */
export const MAX_TAGS = 16;
const TAG_PATTERN = /^[a-z0-9_-]{1,32}$/;

type TagRejection = 'full' | 'invalid';

interface TagInputProps {
  id: string;
  invalid?: boolean;
  value: readonly string[];
  onChange: (tags: string[]) => void;
  /** Why a typed tag was not added; the form says so in its message slot. */
  onReject: (reason: TagRejection) => void;
}

/** A comma or whitespace ends a tag. */
function splitTags(text: string, final: boolean): TokenSplit {
  const parts = text.split(/[\s,]+/);
  const rest = final ? '' : parts.pop() ?? '';
  return { rest, tokens: parts.filter(Boolean) };
}

/**
 * Tags as pills in one field: Enter, a comma or a space adds what was typed,
 * in lower case. Tags stay sorted and distinct, at most `MAX_TAGS`.
 */
export function TagInput({ id, invalid = false, onChange, onReject, value }: TagInputProps) {
  const { t } = useTranslation();

  function add(tokens: string[]): string[] {
    const next = [...value];
    const refused: string[] = [];
    for (const token of tokens) {
      const tag = token.toLowerCase();
      if (!TAG_PATTERN.test(tag)) {
        onReject('invalid');
        refused.push(token);
      } else if (next.includes(tag)) {
        continue;
      } else if (next.length >= MAX_TAGS) {
        onReject('full');
        refused.push(token);
      } else {
        next.push(tag);
      }
    }
    if (next.length !== value.length) onChange(next.sort());
    return refused;
  }

  return (
    <TokenInput
      id={id}
      invalid={invalid}
      maxLength={32}
      onAdd={add}
      onRemove={index => onChange(value.filter((_, each) => each !== index))}
      placeholder={t('tags.placeholder')}
      removeLabel={tag => t('tags.remove', { tag })}
      split={splitTags}
      value={value}
    />
  );
}
