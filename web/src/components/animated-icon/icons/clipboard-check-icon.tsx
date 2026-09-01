import type { Variants } from 'motion/react';

import { motion } from 'motion/react';

interface ClipboardCheckIconProps {
  size: number;
  active: boolean;
}

const CHECK_VARIANTS: Variants = {
  normal: {
    pathLength: 1,
    opacity: 0,
    transition: {
      duration: 0.3,
    },
  },
  animate: {
    pathLength: [0, 1],
    opacity: [0, 1],
    transition: {
      pathLength: { duration: 0.3, ease: 'easeInOut' },
      opacity: { duration: 0.3, ease: 'easeInOut' },
    },
  },
};

function ClipboardCheckIcon({ active, size }: ClipboardCheckIconProps) {
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
      <rect height='4' rx='1' ry='1' width='8' x='8' y='2' />
      <path d='M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2' />
      <motion.path
        animate={active ? 'animate' : 'normal'}
        d='m9 14 2 2 4-4'
        initial='normal'
        style={{ transformOrigin: 'center' }}
        variants={CHECK_VARIANTS}
      />
    </svg>
  );
}

export { ClipboardCheckIcon };
