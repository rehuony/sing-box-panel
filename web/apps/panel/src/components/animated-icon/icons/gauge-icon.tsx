import type { Transition } from 'motion/react';

import { motion } from 'motion/react';

interface GaugeIconProps {
  size: number;
  active: boolean;
}

const DEFAULT_TRANSITION: Transition = {
  type: 'spring',
  stiffness: 160,
  damping: 17,
  mass: 1,
};

function GaugeIcon({ active, size }: GaugeIconProps) {
  return (
    <svg
      fill='none'
      height={size}
      stroke='currentColor'
      strokeLinecap='round'
      strokeLinejoin='round'
      strokeWidth='2'
      viewBox='0 0 24 24'
      width={size}
      xmlns='http://www.w3.org/2000/svg'
    >
      <motion.path
        animate={active ? 'animate' : 'normal'}
        d='m12 14 4-4'
        transition={DEFAULT_TRANSITION}
        variants={{
          animate: { translateX: 0.5, translateY: 3, rotate: 72 },
          normal: {
            translateX: 0,
            rotate: 0,
            translateY: 0,
          },
        }}
      />
      <path d='M3.34 19a10 10 0 1 1 17.32 0' />
    </svg>
  );
}

export { GaugeIcon };
