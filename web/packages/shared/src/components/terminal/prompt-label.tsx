import type { Prompt } from './terminal-entries';

/**
 * What a terminal prints before an input: a shell prompt `user@host in path λ`
 * with user and host, path and symbol in their colours; the code prompt; or the
 * confirmation.
 */
export function PromptLabel({ prompt }: { prompt: Prompt }) {
  switch (prompt.kind) {
    case 'shell':
      return (
        <span aria-hidden='true' className='whitespace-nowrap'>
          <span className='text-[var(--color-highlight-magenta)]'>{`${prompt.user}@${prompt.host}`}</span>
          {' in '}
          <span className='text-[var(--color-highlight-cyan)]'>{prompt.path}</span>
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
