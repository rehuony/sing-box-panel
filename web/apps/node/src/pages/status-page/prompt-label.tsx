import type { PromptKind } from './buffer-entries';

/** What the terminal prints before an input: the shell prompt, the code prompt or the confirmation. */
export function PromptLabel({ host, kind }: { host: string; kind: PromptKind }) {
  switch (kind) {
    case 'shell':
      return (
        <span aria-hidden='true' className='whitespace-nowrap'>
          <span className='text-[var(--color-highlight-magenta)]'>{`console@${host}`}</span>
          {' in '}
          <span className='text-[var(--color-highlight-cyan)]'>~</span>
          {' '}
          <span className='text-[var(--color-highlight-yellow)]'>λ</span>
        </span>
      );
    case 'code':
      return <span aria-hidden='true' className='whitespace-nowrap text-[var(--color-highlight-cyan)]'>code&gt;</span>;
    case 'confirm':
      return <span aria-hidden='true' className='whitespace-nowrap text-[var(--color-danger-text)]'>[y/N]</span>;
  }
}
