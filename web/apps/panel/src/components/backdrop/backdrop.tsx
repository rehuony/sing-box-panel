import type { ReactNode } from 'react';
import type { FieldControl, GlyphFieldHandle } from '@shared/components/glyph-field';

import { useMemo, useRef, useState } from 'react';
import { GlyphField } from '@shared/components/glyph-field';

import { useAuthSession } from '@/stores/auth-session.store';

import { BackdropContext } from './backdrop-context';

/** The height of the sign-in frame's first row, which the field leaves clear. */
const SIGN_IN_ROW = 64;
/** Behind working views the field keeps 30% of its sign-in opacity, so it never competes with data. */
const CALM_OPACITY = 0.3;

/**
 * One glyph field behind every view, so sign-in and the signed-in shell share
 * it: interactive while signed out, calm once signed in, when it mixes the
 * fleet's own values into its text.
 */
export function Backdrop({ children }: { children: ReactNode }) {
  const { status } = useAuthSession();
  const fieldRef = useRef<GlyphFieldHandle>(null);
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const [busy, setBusy] = useState(false);
  const [words, setWords] = useState<readonly string[]>([]);
  const control = useMemo<FieldControl>(() => ({
    anchor: setAnchor,
    pulse: kind => fieldRef.current?.pulse(kind),
    setBusy,
    setWords: next => setWords(current => current.join('\n') === next.join('\n') ? current : next),
  }), []);
  const signedIn = status === 'authenticated';

  // The wrapper isolates the field above the page background.
  return (
    <div className='relative isolate'>
      <GlyphField
        anchor={anchor}
        busy={busy}
        calmOpacity={CALM_OPACITY}
        mode={signedIn ? 'calm' : 'interactive'}
        ref={fieldRef}
        top={signedIn ? 0 : SIGN_IN_ROW}
        words={signedIn ? words : []}
      />
      <BackdropContext value={control}>{children}</BackdropContext>
    </div>
  );
}
