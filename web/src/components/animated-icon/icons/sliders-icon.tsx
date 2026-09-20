import { motion } from 'motion/react';

export function SlidersIcon({ active, size }: { active: boolean; size: number }) {
  return (
    <svg aria-hidden='true' fill='none' height={size} stroke='currentColor' strokeLinecap='round' strokeWidth='2' viewBox='0 0 24 24' width={size}>
      <path d='M3 6h18M3 12h18M3 18h18' />
      {[{ y: 6, x: 8 }, { y: 12, x: 16 }, { y: 18, x: 8 }].map(({ x, y }) => (
        <motion.path key={y} d={`M${x} ${y - 2}v4`} animate={{ x: active ? (x === 8 ? 5 : -5) : 0 }} transition={{ type: 'spring', stiffness: 180, damping: 20 }} />
      ))}
    </svg>
  );
}
