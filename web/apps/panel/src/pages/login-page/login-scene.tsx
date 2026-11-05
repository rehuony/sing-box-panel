import type { MotionValue } from 'motion/react';

import { useEffect, useRef, useState } from 'react';
import { motion, useMotionValue, useSpring, useTransform } from 'motion/react';

import { getReducedMotionSnapshot, usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';

import './login-scene.css';

interface LoginSceneProps {
  hasError: boolean;
  isSubmitting: boolean;
  passwordVisible: boolean;
  focusedField: 'email' | 'password' | null;
  status: 'ready' | 'checking' | 'unavailable';
}

type ScenePose = 'idle' | 'attentive' | 'private' | 'waiting' | 'puzzled';
type CharacterKind = 'main' | 'orange' | 'ink';
type PointerPosition = { x: number; y: number } | null;

interface CharacterProps {
  x: number;
  y: number;
  delay: number;
  width: number;
  height: number;
  pose: ScenePose;
  kind: CharacterKind;
  reducedMotion: boolean;
  pointer: MotionValue<PointerPosition>;
}

function Character({ x, y, width, height, delay, pose, kind, reducedMotion, pointer }: CharacterProps) {
  const [hasEntered, setHasEntered] = useState(false);
  if (reducedMotion && !hasEntered) setHasEntered(true);

  const main = kind === 'main';
  const eyeY = main ? 76 : 51;
  const eyeGap = main ? 23 : 19;
  const center = width / 2;
  const closed = pose === 'private' || pose === 'waiting';
  const puzzled = pose === 'puzzled';
  const transition = { duration: reducedMotion ? 0 : 0.22, ease: 'easeOut' as const };
  const canFollow = !reducedMotion && (pose === 'idle' || pose === 'attentive');
  const gaze = useTransform(pointer, position => {
    if (!canFollow || !position) return { x: 0, y: 0 };
    const dx = position.x - (x + center + (pose === 'attentive' ? 5 : 0));
    const dy = position.y - (y + eyeY);
    const distance = Math.max(70, Math.hypot(dx, dy));
    // Keep pupils inside the eye whites, with less movement near the face.
    return { x: dx / distance * 4.5, y: dy / distance * 6 };
  });
  const gazeX = useTransform(gaze, value => value.x);
  const gazeY = useTransform(gaze, value => value.y);
  const lookX = useSpring(gazeX, { stiffness: 260, damping: 28 });
  const lookY = useSpring(gazeY, { stiffness: 260, damping: 28 });
  const tilt = useTransform(lookX, value => value * (main ? 0.4 : -0.5));
  const lean = pose === 'attentive' ? 3 : puzzled ? -4 : 0;

  useEffect(() => {
    if (canFollow) return;
    lookX.jump(0);
    lookY.jump(0);
  }, [canFollow, lookX, lookY]);

  return (
    <g transform={`translate(${x} ${y})`} className={`login-scene__character login-scene__character--${kind}`}>
      <ellipse className='login-scene__shadow' cx={center} cy={height + 13} rx={width * 0.56} ry='10' />
      {/* Remount to cancel in-flight Motion animations when the OS preference changes. */}
      <motion.g
        key={reducedMotion ? 'still' : 'animated'}
        initial={reducedMotion || hasEntered ? false : { y: 18, rotate: main ? -3 : 3, opacity: 0 }}
        animate={{ y: 0, rotate: 0, opacity: 1 }}
        style={{ originX: 0.5, originY: 1 }}
        transition={{ duration: reducedMotion ? 0 : 0.42, delay: reducedMotion ? 0 : delay }}
        onAnimationComplete={() => setHasEntered(true)}
      >
        <motion.g style={{ rotate: tilt, originX: 0.5, originY: 1 }}>
          <motion.g
            animate={{ rotate: lean, y: pose === 'waiting' ? 3 : 0, scaleY: pose === 'waiting' ? 0.98 : 1 }}
            style={{ originX: 0.5, originY: 1 }}
            transition={transition}
          >
            <path className='login-scene__feet' d={`M${center - 22} ${height - 2}v12m44-12v12`} />
            <rect className='login-scene__body' width={width} height={height} rx={main ? 32 : 28} />
            <path
              className='login-scene__body-shade'
              d={`M${width - 18} 7Q${width} 15 ${width} 32v${height - 64}q0 32-32 32H32q-18 0-25-13h${width - 56}q31 0 31-31Z`}
            />
            <path className='login-scene__highlight' d={`M17 ${main ? 47 : 40}v-9q0-21 22-21h${width * 0.27}`} />

            {/* Switch eyes synchronously while keeping the entire face visible. */}
            <g>
              <motion.g
                initial={false}
                animate={{ x: pose === 'attentive' ? 5 : 0, y: pose === 'waiting' ? 3 : 0 }}
                transition={transition}
              >
                <g>
                  {closed
                    ? (
                        <path
                          className='login-scene__expression login-scene__closed-eyes'
                          d={`M${center - eyeGap - 8} ${eyeY}q8 7 16 0m${eyeGap * 2 - 16} 0q8 7 16 0`}
                        />
                      )
                    : (
                        <>
                          <ellipse className='login-scene__eye' cx={center - eyeGap} cy={eyeY} rx='10' ry='12' />
                          <ellipse className='login-scene__eye' cx={center + eyeGap} cy={eyeY} rx='10' ry='12' />
                          <motion.g style={{ x: reducedMotion ? 0 : lookX, y: reducedMotion ? 0 : lookY }}>
                            <circle className='login-scene__pupil' cx={center - eyeGap} cy={eyeY} r='4.5' />
                            <circle className='login-scene__pupil' cx={center + eyeGap} cy={eyeY} r='4.5' />
                          </motion.g>
                        </>
                      )}
                  {puzzled && (
                    <path
                      className='login-scene__expression'
                      d={`M${center - eyeGap - 5} ${eyeY - 20}l12-4m${eyeGap * 2 - 12} 5l12 4`}
                    />
                  )}
                  <path
                    className='login-scene__expression login-scene__mouth'
                    d={puzzled
                      ? `M${center - 7} ${eyeY + 29}q7-5 14 0`
                      : closed
                        ? `M${center - 5} ${eyeY + 28}h10`
                        : `M${center - 8} ${eyeY + 25}q8 9 16 0`}
                  />
                </g>
              </motion.g>
            </g>
          </motion.g>
        </motion.g>
      </motion.g>
    </g>
  );
}

export function LoginScene({ focusedField, passwordVisible, isSubmitting, hasError, status }: LoginSceneProps) {
  const reducedMotion = usePrefersReducedMotion();
  const [errorSettled, setErrorSettled] = useState(false);
  const illustrationRef = useRef<SVGSVGElement>(null);
  const pointer = useMotionValue<PointerPosition>(null);

  if (!hasError && errorSettled) setErrorSettled(false);

  useEffect(() => {
    if (!hasError) return;
    const timer = window.setTimeout(setErrorSettled, 1100, true);
    return () => window.clearTimeout(timer);
  }, [hasError]);

  const pose: ScenePose = isSubmitting || status !== 'ready'
    ? 'waiting'
    : passwordVisible || focusedField === 'password'
      ? 'private'
      : hasError && !errorSettled
        ? 'puzzled'
        : focusedField !== null
          ? 'attentive'
          : 'idle';

  useEffect(() => {
    pointer.set(null);
    if (reducedMotion) return;

    function resetPointer() {
      pointer.set(null);
    }

    function followPointer(event: PointerEvent) {
      if (getReducedMotionSnapshot() || event.pointerType !== 'mouse'
        || !window.matchMedia('(pointer: fine)').matches) {
        resetPointer();
        return;
      }
      const matrix = illustrationRef.current?.getScreenCTM();
      if (!matrix) return;
      // Convert viewport coordinates into the SVG viewBox, including letterboxing.
      const position = new DOMPoint(event.clientX, event.clientY).matrixTransform(matrix.inverse());
      pointer.set({ x: position.x, y: position.y });
    }

    // Remember the latest target while eyes are closed, so reopening them
    // resumes the gaze without waiting for another mouse movement.
    window.addEventListener('pointermove', followPointer, { passive: true });
    window.addEventListener('blur', resetPointer);
    window.addEventListener('pointercancel', resetPointer);
    document.documentElement.addEventListener('pointerleave', resetPointer);
    return () => {
      window.removeEventListener('pointermove', followPointer);
      window.removeEventListener('blur', resetPointer);
      window.removeEventListener('pointercancel', resetPointer);
      document.documentElement.removeEventListener('pointerleave', resetPointer);
    };
  }, [reducedMotion, pointer]);

  const characterProps = { pose, reducedMotion, pointer };

  return (
    <div className='login-scene' aria-hidden='true'>
      <svg ref={illustrationRef} className='login-scene__illustration' viewBox='0 0 440 320' fill='none' focusable='false'>
        <ellipse className='login-scene__ground' cx='222' cy='286' rx='188' ry='23' />
        <Character {...characterProps} kind='main' x={145} y={35} width={144} height={232} delay={0} />
        <Character {...characterProps} kind='ink' x={51} y={133} width={112} height={143} delay={0.07} />
        <Character {...characterProps} kind='orange' x={280} y={109} width={113} height={167} delay={0.14} />
      </svg>
    </div>
  );
}
