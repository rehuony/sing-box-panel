import type { Prompt } from './terminal-entries';

type ShellPrompt = Extract<Prompt, { kind: 'shell' }>;

/** A shell prompt's scope, `user@host in path`, with user and host and path in their colours. */
export function PromptScope({ prompt }: { prompt: ShellPrompt }) {
  return (
    <span aria-hidden='true' className='whitespace-nowrap'>
      <span className='text-[var(--color-highlight-magenta)]'>{`${prompt.user}@${prompt.host}`}</span>
      {' in '}
      <span className='text-[var(--color-highlight-cyan)]'>{prompt.path}</span>
    </span>
  );
}

/**
 * What a terminal prints before an input: a shell prompt `user@host in path λ`,
 * or its symbol alone where the scope shows elsewhere; the code prompt; or the
 * confirmation. Its kind is on the element, for a terminal that colours one apart.
 */
export function PromptLabel({ prompt, scope = true }: { prompt: Prompt; scope?: boolean }) {
  switch (prompt.kind) {
    case 'shell':
      return (
        <span aria-hidden='true' className='whitespace-nowrap' data-prompt='shell'>
          {scope
            ? (
                <>
                  <PromptScope prompt={prompt} />
                  {' '}
                </>
              )
            : null}
          <span className='text-[var(--color-highlight-yellow)]'>λ</span>
        </span>
      );
    case 'code':
      return <span aria-hidden='true' className='whitespace-nowrap text-[var(--color-highlight-cyan)]' data-prompt='code'>code&gt;</span>;
    case 'confirm':
      return <span aria-hidden='true' className='whitespace-nowrap text-[var(--color-danger-text)]' data-prompt='confirm'>[y/N]</span>;
  }
}
