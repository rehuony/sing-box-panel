import type { FieldControl } from '@shared/components/glyph-field';

import { createContext, use } from 'react';

/** Without a backdrop, such as in isolated tests, views talk to nothing. */
const silent: FieldControl = {
  anchor: () => undefined,
  pulse: () => undefined,
  setBusy: () => undefined,
  setWords: () => undefined,
};

export const BackdropContext = createContext<FieldControl>(silent);

/** What a view tells the glyph field behind the panel. */
export function useBackdrop(): FieldControl {
  return use(BackdropContext);
}
