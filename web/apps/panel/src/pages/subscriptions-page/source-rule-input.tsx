import { useTranslation } from 'react-i18next';

import type { TokenSplit } from '@/components/token-input';

import { TokenInput } from '@/components/token-input';

/** The longest rule the server accepts, in bytes. */
const MAX_RULE_BYTES = 512;

const separator = (character: string) => /[\s,，]/.test(character);

/** Text wrapped in slashes, which the server reads as a regular expression. */
function isPattern(rule: string): boolean {
  return rule.length >= 2 && rule.startsWith('/') && rule.endsWith('/');
}

/**
 * Splits text into filter rules. Whitespace and commas end a rule, except
 * inside a regular expression: from an opening slash up to a closing one
 * that ends the text or is followed by a separator, so a pattern may hold
 * spaces. A line break always ends a rule. Until `final`, the last rule stays
 * being typed.
 */
function splitRules(text: string, final: boolean): TokenSplit {
  const tokens: string[] = [];
  let start = 0;
  while (start < text.length) {
    while (start < text.length && separator(text[start])) start++;
    if (start >= text.length) break;
    let end = start;
    if (text[start] === '/') {
      end = start + 1;
      let closed = false;
      while (end < text.length && text[end] !== '\n') {
        if (text[end] === '\\') {
          end += 2;
          continue;
        }
        if (text[end] === '/' && (end + 1 === text.length || separator(text[end + 1]))) {
          closed = true;
          end++;
          break;
        }
        end++;
      }
      end = Math.min(end, text.length);
      if (!closed && end === text.length && !final) return { rest: text.slice(start), tokens };
    } else {
      while (end < text.length && !separator(text[end])) end++;
      if (end === text.length && !final) return { rest: text.slice(start), tokens };
    }
    tokens.push(text.slice(start, end));
    start = end;
  }
  return { rest: '', tokens };
}

/**
 * A source's include or exclude rules as pills in one field. Rules keep the
 * order they were given and stay distinct; a pattern's slashes read faint,
 * and a rule the server refused reads in danger.
 */
export function SourceRuleInput({ describedBy, id, invalidRules, onChange, value }: {
  id: string;
  value: readonly string[];
  onChange: (rules: string[]) => void;
  /** Indexes of the rules the server refused. */
  invalidRules: readonly number[];
  describedBy?: string;
}) {
  const { t } = useTranslation();

  function add(tokens: string[]): string[] {
    const next = [...value];
    for (const token of tokens) {
      const rule = token.trim();
      if (rule !== '' && !next.includes(rule)) next.push(rule);
    }
    if (next.length !== value.length) onChange(next);
    return [];
  }

  return (
    <TokenInput
      describedBy={describedBy}
      id={id}
      invalid={invalidRules.length > 0}
      maxLength={MAX_RULE_BYTES}
      onAdd={add}
      onRemove={index => onChange(value.filter((_, each) => each !== index))}
      placeholder={t('subscriptions.sources.rulePlaceholder')}
      removeLabel={rule => t('subscriptions.sources.removeRule', { rule })}
      renderToken={rule => isPattern(rule)
        ? (
            <>
              <span className='text-[var(--color-text-faint)]'>/</span>
              {rule.slice(1, -1)}
              <span className='text-[var(--color-text-faint)]'>/</span>
            </>
          )
        : rule}
      split={splitRules}
      tokenInvalid={index => invalidRules.includes(index)}
      value={value}
    />
  );
}
