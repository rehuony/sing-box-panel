import type { ComponentPropsWithoutRef, ElementType } from 'react';

import { act, render } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { AnimatedIcon } from '@/components/animated-icon';

interface MotionElementProps extends ComponentPropsWithoutRef<'svg'> {
  custom?: unknown;
  animate?: unknown;
  initial?: unknown;
  variants?: unknown;
  transition?: unknown;
}

vi.mock('motion/react', async () => {
  const React = await import('react');
  const createMotionElement = (element: ElementType) => (
    {
      animate: _animate,
      custom: _custom,
      initial: _initial,
      transition: _transition,
      variants: _variants,
      ...props
    }: MotionElementProps,
  ) => React.createElement(element, { ...props, 'data-motion-animate': String(_animate) });
  return {
    motion: { path: createMotionElement('path'), svg: createMotionElement('svg') },
  };
});

function installMotionPreference(initialValue: boolean) {
  let matches = initialValue;
  const listeners = new Set<EventListener>();
  const mediaQueryList = {
    get matches() {
      return matches;
    },
    media: '(prefers-reduced-motion: reduce)',
    onchange: null,
    addEventListener: (_type: string, listener: EventListener) => listeners.add(listener),
    removeEventListener: (_type: string, listener: EventListener) => listeners.delete(listener),
  } as unknown as MediaQueryList;
  vi.stubGlobal('matchMedia', vi.fn(() => mediaQueryList));
  return (nextValue: boolean) => {
    matches = nextValue;
    const event = new Event('change') as MediaQueryListEvent;
    Object.defineProperty(event, 'matches', { value: matches });
    act(() => listeners.forEach(listener => listener(event)));
  };
}

beforeEach(() => {
  installMotionPreference(false);
});

afterEach(() => vi.unstubAllGlobals());

describe('animatedIcon', () => {
  it('starts and stops only from its controlled active state', () => {
    const view = render(<AnimatedIcon active={false} name='dashboard' />);
    expect(view.container.querySelector('[data-motion-animate="normal"]')).not.toBeNull();

    view.rerender(<AnimatedIcon active name='dashboard' />);
    expect(view.container.querySelector('[data-motion-animate="animate"]')).not.toBeNull();

    view.rerender(<AnimatedIcon active={false} name='dashboard' />);
    expect(view.container.querySelector('[data-motion-animate="normal"]')).not.toBeNull();
  });

  it('renders a static fallback when reduced motion is requested', () => {
    installMotionPreference(true);
    const view = render(<AnimatedIcon active name='configuration' />);

    expect(view.container.querySelector('.lucide-sliders-horizontal')).not.toBeNull();
    expect(view.container.querySelector('[data-motion-animate="animate"]')).toBeNull();
  });
});
