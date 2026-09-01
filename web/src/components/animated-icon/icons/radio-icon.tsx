import type { Variants } from 'motion/react';

import { motion } from 'motion/react';

interface RadioIconProps {
  size: number;
  active: boolean;
}

const VARIANTS: Variants = {
  normal: {
    opacity: 1,
    transition: {
      duration: 0.4,
    },
  },
  animate: (index: number) => ({
    opacity: [1, 0, 1],
    transition: {
      type: 'tween',
      duration: 0.54,
      ease: 'easeOut',
      delay: index * 0.08,
      times: [0, 0.56, 1],
    },
  }),
};

function RadioIcon({ active, size }: RadioIconProps) {
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
        custom={1}
        d='M4.9 19.1C1 15.2 1 8.8 4.9 4.9'
        initial={{ opacity: 1 }}
        variants={VARIANTS}
      />
      <motion.path
        animate={active ? 'animate' : 'normal'}
        custom={0}
        d='M7.8 16.2c-2.3-2.3-2.3-6.1 0-8.5'
        initial={{ opacity: 1 }}
        variants={VARIANTS}
      />
      <circle cx='12' cy='12' r='2' />
      <motion.path
        animate={active ? 'animate' : 'normal'}
        custom={0}
        d='M16.2 7.8c2.3 2.3 2.3 6.1 0 8.5'
        initial={{ opacity: 1 }}
        variants={VARIANTS}
      />
      <motion.path
        animate={active ? 'animate' : 'normal'}
        custom={1}
        d='M19.1 4.9C23 8.8 23 15.1 19.1 19'
        initial={{ opacity: 1 }}
        variants={VARIANTS}
      />
    </svg>
  );
}

export { RadioIcon };
