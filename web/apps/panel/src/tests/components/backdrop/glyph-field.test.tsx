import { act, render } from '@testing-library/react';
import { GlyphField } from '@shared/components/glyph-field';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

/** A 2D context that records what the field clears; text measures 7px a character. */
function recordingContext() {
  const cleared: [number, number, number, number][] = [];
  const context = {
    beginPath: () => undefined,
    clearRect: (x: number, y: number, width: number, height: number) => cleared.push([x, y, width, height]),
    clip: () => undefined,
    fillRect: () => undefined,
    fillText: () => undefined,
    measureText: (text: string) => ({ width: text.length * 7 }),
    rect: () => undefined,
    restore: () => undefined,
    save: () => undefined,
    setTransform: () => undefined,
  };
  return { cleared, context: context as unknown as CanvasRenderingContext2D };
}

function wholeWindow([, , width, height]: [number, number, number, number]) {
  return width >= window.innerWidth && height >= window.innerHeight;
}

describe('calm glyph field', () => {
  let recording: ReturnType<typeof recordingContext>;

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'requestAnimationFrame', 'cancelAnimationFrame', 'performance'] });
    recording = recordingContext();
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(recording.context);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    document.documentElement.removeAttribute('style');
  });

  it('types its rewrites by redrawing their boxes alone, without animation frames', () => {
    render(<GlyphField mode='calm' words={['edge-01', '203.0.113.7']} />);
    // Placing the field and clearing its anchor settle within a frame.
    act(() => vi.advanceTimersByTime(100));
    const frames = vi.spyOn(globalThis, 'requestAnimationFrame');
    recording.cleared.length = 0;

    act(() => vi.advanceTimersByTime(6000));

    expect(frames).not.toHaveBeenCalled();
    expect(recording.cleared.length).toBeGreaterThan(0);
    expect(recording.cleared.some(wholeWindow)).toBe(false);
  });

  it('draws again only when a write to the root changes the colours it paints with', async () => {
    render(<GlyphField mode='calm' words={[]} />);
    act(() => vi.advanceTimersByTime(100));
    recording.cleared.length = 0;

    // A menu's scroll lock writes the root's style too.
    document.documentElement.style.setProperty('scrollbar-gutter', 'stable');
    await act(() => Promise.resolve());
    expect(recording.cleared.some(wholeWindow)).toBe(false);

    document.documentElement.style.setProperty('--primary', '#2563eb');
    await act(() => Promise.resolve());
    expect(recording.cleared.some(wholeWindow)).toBe(true);
  });
});
