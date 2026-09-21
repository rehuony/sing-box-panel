import type { ComponentPropsWithoutRef } from 'react';

import { useSyncExternalStore } from 'react';
import {
  Activity as StaticActivityIcon,
  Boxes as StaticBoxesIcon,
  Gauge as StaticGaugeIcon,
  Radio as StaticRadioIcon,
  Settings as StaticSettingsIcon,
  SlidersHorizontal as StaticSlidersIcon,
} from 'lucide-react';

import { BoxesIcon } from './icons/boxes-icon';
import { GaugeIcon } from './icons/gauge-icon';
import { RadioIcon } from './icons/radio-icon';
import { SlidersIcon } from './icons/sliders-icon';
import { ActivityIcon } from './icons/activity-icon';
import { SettingsIcon } from './icons/settings-icon';

const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)';

const ICONS = {
  configuration: {
    animated: SlidersIcon,
    fallback: StaticSlidersIcon,
  },
  panel: {
    animated: SettingsIcon,
    fallback: StaticSettingsIcon,
  },
  cores: {
    animated: BoxesIcon,
    fallback: StaticBoxesIcon,
  },
  dashboard: {
    animated: GaugeIcon,
    fallback: StaticGaugeIcon,
  },
  observability: {
    animated: ActivityIcon,
    fallback: StaticActivityIcon,
  },
  subscriptions: {
    animated: RadioIcon,
    fallback: StaticRadioIcon,
  },
} as const;

export type AnimatedIconName = keyof typeof ICONS;

export interface AnimatedIconProps extends Omit<ComponentPropsWithoutRef<'span'>, 'aria-label' | 'children'> {
  size?: number;
  active: boolean;
  name: AnimatedIconName;
}

function subscribeToReducedMotion(onStoreChange: () => void): () => void {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return () => undefined;
  }

  const mediaQuery = window.matchMedia(REDUCED_MOTION_QUERY);
  mediaQuery.addEventListener('change', onStoreChange);

  return () => mediaQuery.removeEventListener('change', onStoreChange);
}

function getReducedMotionSnapshot(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return true;
  }

  return window.matchMedia(REDUCED_MOTION_QUERY).matches;
}

function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(subscribeToReducedMotion, getReducedMotionSnapshot, () => true);
}

function AnimatedIcon({ active, className, name, size = 20, ...props }: AnimatedIconProps) {
  const prefersReducedMotion = usePrefersReducedMotion();
  const definition = ICONS[name];

  const RegistryIcon = definition.animated;
  const FallbackIcon = definition.fallback;

  return (
    <span
      {...props}
      aria-hidden='true'
      className={className}
      data-active={active || undefined}
      data-animated-icon={name}
      data-slot='animated-icon'
    >
      {prefersReducedMotion
        ? <FallbackIcon aria-hidden='true' focusable='false' size={size} />
        : <RegistryIcon active={active} size={size} />}
    </span>
  );
}

export { AnimatedIcon };
