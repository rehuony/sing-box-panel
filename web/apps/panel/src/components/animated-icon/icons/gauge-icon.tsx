import { motion } from 'motion/react';

interface GaugeIconProps {
  size: number;
  active: boolean;
}

/** lucide's gauge, its needle swinging up and back on activation. */
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
        animate={active ? { rotate: [0, -50, 10, 0] } : { rotate: 0 }}
        d='m12 14 4-4'
        style={{ originX: '12px', originY: '14px' }}
        transition={{ duration: 0.6, ease: 'easeInOut' }}
      />
      <path d='M3.34 19a10 10 0 1 1 17.32 0' />
    </svg>
  );
}

export { GaugeIcon };
