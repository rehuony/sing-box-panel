import type { Ref } from 'react';

import { useEffect, useImperativeHandle, useRef } from 'react';

import type { FieldDepth } from './field-tokens';

import { fieldToken } from './field-tokens';

/** What the field answers besides the pointer. */
export type FieldPulse = 'key' | 'error' | 'success' | 'refresh';

/**
 * Interactive behind sign-in; calm behind the signed-in view: fainter, still
 * under the pointer, and rewriting slowly with the node's own values.
 */
export type FieldMode = 'interactive' | 'calm';

export interface ConsoleFieldHandle {
  pulse: (kind: FieldPulse) => void;
}

/** What a view tells the field: what to keep clear, what happened, what it shows. */
export interface FieldControl {
  setBusy: (busy: boolean) => void;
  pulse: (kind: FieldPulse) => void;
  setWords: (words: readonly string[]) => void;
  /** A ref for the element the field centers on and keeps clear of. */
  anchor: (element: HTMLElement | null) => void;
}

interface ConsoleFieldProps {
  /** While true, glyphs keep streaming toward the anchor. */
  busy: boolean;
  mode: FieldMode;
  /** Values of this node the field mixes into its text in calm mode. */
  words: readonly string[];
  /** The element the field centers on and keeps clear of; the window's center while null. */
  anchor: HTMLElement | null;
  ref?: Ref<ConsoleFieldHandle>;
}

type Tone = 'accent' | 'success' | 'danger';

interface Glyph {
  x: number;
  y: number;
  tone: Tone;
  vx: number;
  vy: number;
  heat: number;
  pull: number;
  text: string;
  /** Its tilt at rest, in radians. */
  angle: number;
  homeX: number;
  homeY: number;
  /** Whether it moves aside, rather than fading, when the anchor covers it. */
  keep: boolean;
  /** Width of its whole text in its layer's font. */
  width: number;
  hidden: boolean;
  /** Where it was placed; home again whenever the anchor leaves it clear. */
  originX: number;
  originY: number;
  depth: FieldDepth;
  visibility: number;
  /** When it began typing its current text, or null once typed. */
  typing: number | null;
}

interface Palette {
  base: string;
  dark: boolean;
  family: string;
  tones: Record<Tone, string>;
}

interface Wave {
  tone: Tone;
  heat: number;
  start: number;
}

/**
 * Three layers, far to near: smaller and fainter text further back, pushed
 * less by the pointer, so the field has depth and parallax.
 */
const LAYERS = [
  { alpha: { dark: 0.16, light: 0.22 }, push: 0.4, share: 0.42, size: 10 },
  { alpha: { dark: 0.28, light: 0.34 }, push: 0.7, share: 0.36, size: 12 },
  { alpha: { dark: 0.4, light: 0.46 }, push: 1, share: 0.22, size: 14 },
] as const;

// Placement: best-candidate sampling per layer keeps the field random but
// without large holes; few candidates let neighbours overlap now and then.
const AREA_PER_GLYPH = 5000;
const CANDIDATES = 5;
const ANISOTROPY = 2.4;
const FIRST_ROW = 64;
const EDGE = 4;
const CLEAR_X = 16;
const CLEAR_Y = 14;
// A growing anchor pushes some covered glyphs out to its edge and fades the rest.
const KEEP_DISPLACED = 0.4;
const RELOCATE_STEP = 8;
const RELOCATE_LIMIT = 80;
/** A moved glyph closer than this to a shown neighbour of its layer fades instead. */
const CROWD_DISTANCE = 22;
// Tilt: most glyphs lean a little, some lean more; a push leans them further.
const TILT = 0.21;
const STEEP_TILT = 0.42;
const STEEP_SHARE = 0.15;
const TILT_PER_PIXEL = 0.006;
const MAXIMUM_PUSH_TILT = 0.35;
// Motion, in pixels and seconds; a damped spring returns every glyph home.
const SPRING = 48;
const DAMPING = 13;
const POINTER_RADIUS = 130;
const POINTER_FORCE = 2600;
const PULL_FORCE = 4000;
const PULL_SECONDS = 0.5;
const PULL_RANGE = 520;
const KEY_GLYPHS = 5;
const SHOCK_RADIUS = 380;
const SHOCK_SPEED = 620;
const WAVE_SPEED = 1600;
const WAVE_BAND = 90;
const REFRESH_HEAT = 0.35;
const HEAT_DECAY = 1.4;
const MAXIMUM_STEP = 0.05;
// Calm mode fades the field and favours the node's own values.
const CALM_ALPHA = 0.6;
const ALPHA_RATE = 2;
const VISIBILITY_RATE = 4;
const WORD_SHARE = 0.5;
const WORD_BURST = 24;
const WORD_BURST_GAP_MS = 60;
// Background life: now and then one glyph types a new text, a cursor at its end.
const REWRITE_MS = { calm: 1100, interactive: 420 } as const;
const TYPE_MS = 32;
const CURSOR_LINGER_MS = 360;
const BUSY_MS = 160;

function readPalette(): Palette {
  const root = document.documentElement;
  const style = getComputedStyle(root);
  const read = (name: string) => style.getPropertyValue(name).trim();
  return {
    base: read('--color-text-faint'),
    dark: root.classList.contains('dark') || root.dataset.resolvedTheme === 'dark',
    family: read('--font-data') || 'monospace',
    tones: { accent: read('--primary'), danger: read('--color-danger'), success: read('--color-success') },
  };
}

function layerFont(palette: Palette, depth: FieldDepth): string {
  return `400 ${LAYERS[depth].size}px ${palette.family}`;
}

function restingTilt(): number {
  if (Math.random() < STEEP_SHARE) return (Math.random() * 2 - 1) * STEEP_TILT;
  // Two uniform draws make a triangle: most tilts stay near level.
  return (Math.random() + Math.random() - 1) * TILT;
}

/** Half the width and height of the box a glyph covers at its tilt. */
function extents(glyph: Pick<Glyph, 'angle' | 'depth' | 'width'>) {
  const halfWidth = glyph.width / 2;
  const halfHeight = LAYERS[glyph.depth].size * 0.6;
  const cos = Math.abs(Math.cos(glyph.angle));
  const sin = Math.abs(Math.sin(glyph.angle));
  return { x: halfWidth * cos + halfHeight * sin, y: halfWidth * sin + halfHeight * cos };
}

function covers(rect: DOMRect | undefined, x: number, y: number, extent: { x: number; y: number }): boolean {
  return rect !== undefined
    && x + extent.x > rect.left - CLEAR_X && x - extent.x < rect.right + CLEAR_X
    && y + extent.y > rect.top - CLEAR_Y && y - extent.y < rect.bottom + CLEAR_Y;
}

function warm(glyph: Glyph, amount: number, tone: Tone) {
  if (amount < glyph.heat && tone === glyph.tone) return;
  glyph.heat = Math.max(glyph.heat, amount);
  glyph.tone = tone;
}

function approach(value: number, target: number, step: number): number {
  return value < target ? Math.min(target, value + step) : Math.max(target, value - step);
}

interface Engine {
  takeWords: () => void;
  changeMode: () => void;
  clearAnchor: () => void;
  pulse: (kind: FieldPulse) => void;
}

/**
 * The console's backdrop: protocol names, settings, addresses and log
 * fragments in the data face, scattered over three tilted layers. Behind
 * sign-in the pointer pushes and leans them aside, keystrokes stream them
 * toward the password, and the outcome sends a wave or a shock through them.
 * Behind the signed-in view it is calm: fainter, still under the pointer,
 * pulsing once per status refresh and rewriting itself with the node's own
 * values. Glyphs the anchor covers move to its edge or fade. Under reduced
 * motion it is a still field that only changes colour.
 */
export function ConsoleField({ anchor, busy, mode, ref, words }: ConsoleFieldProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const engineRef = useRef<Engine | null>(null);
  const settingsRef = useRef({ anchor, busy, mode, words });
  settingsRef.current = { anchor, busy, mode, words };

  useImperativeHandle(ref, () => ({ pulse: kind => engineRef.current?.pulse(kind) }), []);

  useEffect(() => {
    const canvas = canvasRef.current;
    const context = canvas?.getContext('2d');
    if (!canvas || !context) return;
    const motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
    let reducedMotion = motionQuery.matches;
    let palette = readPalette();
    let glyphs: Glyph[] = [];
    let width = 0;
    let height = 0;
    let ratio = 1;
    let pointer: { x: number; y: number } | null = null;
    let wave: Wave | null = null;
    let fieldAlpha = settingsRef.current.mode === 'calm' ? CALM_ALPHA : 1;
    let frame = 0;
    let lastStep = 0;

    function anchorRect() {
      return settingsRef.current.anchor?.getBoundingClientRect();
    }

    function center() {
      const rect = anchorRect();
      return rect ? { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 } : { x: width / 2, y: height / 2 };
    }

    function outside(x: number, y: number, extent: { x: number; y: number }) {
      return x - extent.x < EDGE || x + extent.x > width - EDGE || y - extent.y < FIRST_ROW || y + extent.y > height;
    }

    /** Text for depth: in calm mode often one of the node's values that fits the layer. */
    function nextToken(depth: FieldDepth): string {
      const { mode, words } = settingsRef.current;
      if (mode === 'calm' && Math.random() < WORD_SHARE) {
        const limit = depth === 2 ? 10 : depth === 1 ? 24 : Number.POSITIVE_INFINITY;
        // Each value shows once at a time; a repeated one reads as a glitch.
        const shown = new Set(glyphs.filter(glyph => !glyph.hidden).map(glyph => glyph.text));
        const fitting = words.filter(word => word.length <= limit && !shown.has(word));
        if (fitting.length > 0) return fitting[Math.floor(Math.random() * fitting.length)]!;
      }
      return fieldToken(depth);
    }

    function draw(now = performance.now()) {
      context!.setTransform(ratio, 0, 0, ratio, 0, 0);
      context!.clearRect(0, 0, width, height);
      context!.textBaseline = 'middle';
      context!.textAlign = 'left';
      let depth = -1;
      for (const glyph of glyphs) {
        if (glyph.visibility < 0.01) continue;
        if (glyph.depth !== depth) {
          depth = glyph.depth;
          context!.font = layerFont(palette, glyph.depth);
        }
        const layer = LAYERS[glyph.depth];
        const base = palette.dark ? layer.alpha.dark : layer.alpha.light;
        const hot = glyph.heat > 0.02;
        const alpha = hot ? Math.min(1, base + glyph.heat * 0.8) : base;
        context!.globalAlpha = alpha * fieldAlpha * glyph.visibility;
        context!.fillStyle = hot ? palette.tones[glyph.tone] : palette.base;
        const shift = (glyph.x - glyph.homeX) * TILT_PER_PIXEL;
        const lean = reducedMotion ? 0 : Math.max(-MAXIMUM_PUSH_TILT, Math.min(MAXIMUM_PUSH_TILT, shift));
        const cos = Math.cos(glyph.angle + lean);
        const sin = Math.sin(glyph.angle + lean);
        context!.setTransform(ratio * cos, ratio * sin, -ratio * sin, ratio * cos, ratio * glyph.x, ratio * glyph.y);
        const left = -glyph.width / 2;
        if (glyph.typing === null) {
          context!.fillText(glyph.text, left, 0);
        } else {
          const shown = glyph.text.slice(0, Math.max(0, Math.floor((now - glyph.typing) / TYPE_MS)));
          context!.fillText(shown, left, 0);
          const size = layer.size;
          context!.fillRect(left + context!.measureText(shown).width + 1, -size * 0.45, size * 0.5, size * 0.9);
        }
      }
      context!.setTransform(ratio, 0, 0, ratio, 0, 0);
      context!.globalAlpha = 1;
    }

    function layout() {
      ratio = Math.min(window.devicePixelRatio || 1, 2);
      width = window.innerWidth;
      height = window.innerHeight;
      canvas!.width = Math.round(width * ratio);
      canvas!.height = Math.round(height * ratio);
      const rect = anchorRect();
      const total = Math.round(width * Math.max(0, height - FIRST_ROW) / AREA_PER_GLYPH);
      glyphs = [];
      LAYERS.forEach((layer, index) => {
        const depth = index as FieldDepth;
        context!.font = layerFont(palette, depth);
        const placed: Glyph[] = [];
        for (let count = Math.round(total * layer.share); count > 0; count--) {
          const text = nextToken(depth);
          const shape = { angle: restingTilt(), depth, width: context!.measureText(text).width };
          const extent = extents(shape);
          let best: { x: number; y: number; score: number } | null = null;
          for (let candidate = 0; candidate < CANDIDATES; candidate++) {
            const x = extent.x + EDGE + Math.random() * (width - 2 * (extent.x + EDGE));
            const y = FIRST_ROW + extent.y + Math.random() * (height - FIRST_ROW - 2 * extent.y);
            if (outside(x, y, extent) || covers(rect, x, y, extent)) continue;
            let score = Number.POSITIVE_INFINITY;
            for (const other of placed) {
              score = Math.min(score, ((x - other.homeX) / ANISOTROPY) ** 2 + (y - other.homeY) ** 2);
            }
            if (best === null || score > best.score) best = { score, x, y };
          }
          if (best === null) continue;
          placed.push({
            ...shape, heat: 0, hidden: false, homeX: best.x, homeY: best.y, keep: Math.random() < KEEP_DISPLACED,
            originX: best.x, originY: best.y, pull: 0, text, tone: 'accent', typing: null, visibility: 1,
            vx: 0, vy: 0, x: best.x, y: best.y,
          });
        }
        glyphs.push(...placed);
      });
      draw();
    }

    /** Whether a glyph moved to x, y would sit on a shown neighbour of its layer. */
    function crowded(glyph: Glyph, x: number, y: number) {
      return glyphs.some(other => other !== glyph && !other.hidden && other.depth === glyph.depth
        && ((x - other.homeX) / ANISOTROPY) ** 2 + (y - other.homeY) ** 2 < CROWD_DISTANCE ** 2);
    }

    /** Moves glyphs the anchor covers to its edge, or fades them, and brings back the ones it uncovers. */
    function clearAnchor() {
      const rect = anchorRect();
      const middle = center();
      for (const glyph of glyphs) {
        const extent = extents(glyph);
        if (!covers(rect, glyph.originX, glyph.originY, extent)) {
          glyph.homeX = glyph.originX;
          glyph.homeY = glyph.originY;
          glyph.hidden = false;
          continue;
        }
        let x = glyph.originX;
        let y = glyph.originY;
        const length = Math.hypot(x - middle.x, y - middle.y) || 1;
        const dx = (x - middle.x) / length;
        const dy = (y - middle.y) / length;
        for (let step = 0; step < RELOCATE_LIMIT && covers(rect, x, y, extent); step++) {
          x += dx * RELOCATE_STEP;
          y += dy * RELOCATE_STEP;
        }
        glyph.hidden = !glyph.keep || covers(rect, x, y, extent) || outside(x, y, extent) || crowded(glyph, x, y);
        if (!glyph.hidden) {
          glyph.homeX = x;
          glyph.homeY = y;
        }
      }
      if (reducedMotion) {
        for (const glyph of glyphs) {
          glyph.x = glyph.homeX;
          glyph.y = glyph.homeY;
          glyph.visibility = glyph.hidden ? 0 : 1;
        }
        draw();
      }
      wake();
    }

    /** Gives a glyph a new text that fits where it rests, typed out from start. */
    function rewrite(glyph: Glyph, start: number) {
      const rect = anchorRect();
      context!.font = layerFont(palette, glyph.depth);
      for (let attempt = 0; attempt < 3; attempt++) {
        const text = nextToken(glyph.depth);
        const shape = { angle: glyph.angle, depth: glyph.depth, width: context!.measureText(text).width };
        const extent = extents(shape);
        if (outside(glyph.homeX, glyph.homeY, extent) || covers(rect, glyph.homeX, glyph.homeY, extent)) continue;
        glyph.text = text;
        glyph.width = shape.width;
        glyph.typing = start;
        warm(glyph, 0.5, 'accent');
        return;
      }
    }

    function step(now: number) {
      frame = 0;
      const seconds = Math.min((now - lastStep) / 1000, MAXIMUM_STEP);
      lastStep = now;
      const target = center();
      const front = wave ? (now - wave.start) / 1000 * WAVE_SPEED : 0;
      const alphaTarget = settingsRef.current.mode === 'calm' ? CALM_ALPHA : 1;
      fieldAlpha = approach(fieldAlpha, alphaTarget, ALPHA_RATE * seconds);
      let active = fieldAlpha !== alphaTarget;
      for (const glyph of glyphs) {
        const push = LAYERS[glyph.depth].push;
        if (reducedMotion) {
          glyph.x = glyph.homeX;
          glyph.y = glyph.homeY;
          glyph.visibility = glyph.hidden ? 0 : 1;
        } else {
          let ax = -SPRING * (glyph.x - glyph.homeX) - DAMPING * glyph.vx;
          let ay = -SPRING * (glyph.y - glyph.homeY) - DAMPING * glyph.vy;
          if (pointer && !glyph.hidden) {
            const dx = glyph.x - pointer.x;
            const dy = glyph.y - pointer.y;
            const distance = Math.hypot(dx, dy);
            if (distance < POINTER_RADIUS && distance > 0.01) {
              const strength = (1 - distance / POINTER_RADIUS) ** 2;
              ax += dx / distance * strength * POINTER_FORCE * push;
              ay += dy / distance * strength * POINTER_FORCE * push;
              warm(glyph, strength, 'accent');
            }
          }
          if (glyph.pull > 0) {
            const dx = target.x - glyph.x;
            const dy = target.y - glyph.y;
            const distance = Math.hypot(dx, dy) || 1;
            ax += dx / distance * PULL_FORCE;
            ay += dy / distance * PULL_FORCE;
            glyph.pull -= seconds;
          }
          glyph.vx += ax * seconds;
          glyph.vy += ay * seconds;
          glyph.x += glyph.vx * seconds;
          glyph.y += glyph.vy * seconds;
          glyph.visibility = approach(glyph.visibility, glyph.hidden ? 0 : 1, VISIBILITY_RATE * seconds);
        }
        if (wave && Math.abs(front - Math.hypot(glyph.homeX - target.x, glyph.homeY - target.y)) < WAVE_BAND) {
          warm(glyph, wave.heat, wave.tone);
        }
        if (glyph.typing !== null && now - glyph.typing > glyph.text.length * TYPE_MS + CURSOR_LINGER_MS) {
          glyph.typing = null;
        }
        glyph.heat = Math.max(0, glyph.heat - HEAT_DECAY * seconds);
        const settled = glyph.heat <= 0.01 && glyph.pull <= 0 && glyph.typing === null
          && glyph.visibility === (glyph.hidden ? 0 : 1)
          && Math.abs(glyph.vx) + Math.abs(glyph.vy) <= 2
          && Math.abs(glyph.x - glyph.homeX) + Math.abs(glyph.y - glyph.homeY) <= 0.5;
        if (!settled) active = true;
      }
      if (wave && front > Math.hypot(width, height) + WAVE_BAND) wave = null;
      draw(now);
      if (active || wave) frame = requestAnimationFrame(step);
    }

    function wake() {
      if (frame !== 0 || document.hidden) return;
      lastStep = performance.now();
      frame = requestAnimationFrame(step);
    }

    function visibleGlyphs() {
      return glyphs.filter(glyph => !glyph.hidden);
    }

    function streamToward(count: number) {
      const target = center();
      const near = visibleGlyphs().filter(glyph => glyph.depth > 0
        && Math.hypot(glyph.homeX - target.x, glyph.homeY - target.y) < PULL_RANGE);
      for (let index = 0; index < count && near.length > 0; index++) {
        const glyph = near.splice(Math.floor(Math.random() * near.length), 1)[0]!;
        if (!reducedMotion) glyph.pull = PULL_SECONDS;
        warm(glyph, 1, 'accent');
      }
    }

    const engine: Engine = {
      changeMode() {
        pointer = null;
        wake();
      },
      clearAnchor,
      pulse(kind) {
        if (kind === 'key') {
          streamToward(KEY_GLYPHS);
        } else if (kind === 'success') {
          wave = { heat: 1, start: performance.now(), tone: 'success' };
        } else if (kind === 'refresh') {
          wave = { heat: REFRESH_HEAT, start: performance.now(), tone: 'accent' };
        } else {
          const target = center();
          for (const glyph of visibleGlyphs()) {
            const dx = glyph.x - target.x;
            const dy = glyph.y - target.y;
            const distance = Math.hypot(dx, dy) || 1;
            if (distance > SHOCK_RADIUS) continue;
            const strength = 1 - distance / SHOCK_RADIUS;
            if (!reducedMotion) {
              const push = LAYERS[glyph.depth].push;
              glyph.vx += dx / distance * strength * SHOCK_SPEED * push;
              glyph.vy += dy / distance * strength * SHOCK_SPEED * push;
            }
            warm(glyph, strength, 'danger');
          }
        }
        wake();
      },
      takeWords() {
        if (reducedMotion || settingsRef.current.words.length === 0) return;
        const candidates = visibleGlyphs();
        const now = performance.now();
        for (let index = 0; index < WORD_BURST && candidates.length > 0; index++) {
          const glyph = candidates.splice(Math.floor(Math.random() * candidates.length), 1)[0]!;
          rewrite(glyph, now + index * WORD_BURST_GAP_MS);
        }
        wake();
      },
    };
    engineRef.current = engine;

    function followPointer(event: PointerEvent) {
      if (reducedMotion || event.pointerType !== 'mouse' || settingsRef.current.mode === 'calm') return;
      pointer = { x: event.clientX, y: event.clientY };
      wake();
    }
    function releasePointer() {
      pointer = null;
      wake();
    }
    function changeMotion() {
      reducedMotion = motionQuery.matches;
      pointer = null;
      layout();
      clearAnchor();
    }
    function changeTheme() {
      palette = readPalette();
      draw();
    }
    function changeVisibility() {
      if (document.hidden && frame !== 0) {
        cancelAnimationFrame(frame);
        frame = 0;
      } else {
        wake();
      }
    }
    function relayout() {
      layout();
      clearAnchor();
    }

    layout();
    // The data face may load late; place again so measured widths match it.
    void document.fonts?.ready.then(relayout);
    let rewriteDue = 0;
    const rewriter = window.setInterval(() => {
      if (reducedMotion || document.hidden || glyphs.length === 0) return;
      const now = performance.now();
      if (now < rewriteDue) return;
      rewriteDue = now + REWRITE_MS[settingsRef.current.mode];
      const shown = visibleGlyphs();
      if (shown.length > 0) rewrite(shown[Math.floor(Math.random() * shown.length)]!, now);
      wake();
    }, REWRITE_MS.interactive);
    const stream = window.setInterval(() => {
      if (!settingsRef.current.busy || document.hidden) return;
      streamToward(2);
      wake();
    }, BUSY_MS);
    const themeObserver = new MutationObserver(changeTheme);
    themeObserver.observe(document.documentElement, { attributeFilter: ['class', 'data-resolved-theme', 'style'] });
    window.addEventListener('resize', relayout);
    window.addEventListener('pointermove', followPointer, { passive: true });
    window.addEventListener('blur', releasePointer);
    document.documentElement.addEventListener('pointerleave', releasePointer);
    document.addEventListener('visibilitychange', changeVisibility);
    motionQuery.addEventListener('change', changeMotion);
    return () => {
      cancelAnimationFrame(frame);
      window.clearInterval(rewriter);
      window.clearInterval(stream);
      themeObserver.disconnect();
      window.removeEventListener('resize', relayout);
      window.removeEventListener('pointermove', followPointer);
      window.removeEventListener('blur', releasePointer);
      document.documentElement.removeEventListener('pointerleave', releasePointer);
      document.removeEventListener('visibilitychange', changeVisibility);
      motionQuery.removeEventListener('change', changeMotion);
      engineRef.current = null;
    };
  }, []);

  // The anchor changes with the view and its size with its content.
  useEffect(() => {
    engineRef.current?.clearAnchor();
    if (!anchor || typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(() => engineRef.current?.clearAnchor());
    observer.observe(anchor);
    return () => observer.disconnect();
  }, [anchor]);

  useEffect(() => {
    engineRef.current?.changeMode();
  }, [mode]);

  useEffect(() => {
    engineRef.current?.takeWords();
  }, [words]);

  return <canvas aria-hidden='true' className='pointer-events-none fixed inset-0 -z-10 size-full' ref={canvasRef} />;
}
