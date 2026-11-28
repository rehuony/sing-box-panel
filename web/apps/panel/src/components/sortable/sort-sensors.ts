import type { RefObject } from 'react';
import type { KeyboardCodes, Sensor, SensorOptions } from '@dnd-kit/core';

import { useLayoutEffect, useMemo, useRef } from 'react';
import { sortableKeyboardCoordinates } from '@dnd-kit/sortable';
import { KeyboardSensor, MouseSensor, TouchSensor, useSensor, useSensors } from '@dnd-kit/core';

/** Whether a sensor refuses to start a drag for now. */
interface Lock {
  locked?: RefObject<boolean>;
}

/** F2 picks an item up and drops it, Escape cancels. */
const F2_CODES: KeyboardCodes = { start: ['F2'], end: ['F2'], cancel: ['Escape'] };

/**
 * A sensor that starts no drag while its lock holds. The lock is read as a
 * drag would start, so locking re-renders none of the sortables.
 */
function lockable<T extends SensorOptions>(sensor: Sensor<T>): Sensor<T & Lock> {
  class Locked extends (sensor as Sensor<T & Lock>) {}
  Locked.activators = sensor.activators.map(({ eventName, handler }) => ({
    eventName,
    handler: (event, options: T & Lock, context) => !options.locked?.current && handler(event, options, context),
  }));
  return Locked;
}

const LockableMouseSensor = lockable(MouseSensor);
const LockableTouchSensor = lockable(TouchSensor);
const LockableKeyboardSensor = lockable(KeyboardSensor);

/**
 * The sensors a sortable list drags by: the mouse once it moves 6px, a touch
 * held 180ms and the keyboard with the given keys, F2 by default. None starts
 * a drag while `locked` holds, such as while a new order saves. The keys must
 * be a stable value.
 */
export function useSortSensors(locked = false, keyboardCodes = F2_CODES) {
  const lockRef = useRef(locked);
  useLayoutEffect(() => {
    lockRef.current = locked;
  });
  // Options that keep their identity keep the context's activators, so a render re-renders no sortable.
  const mouse = useMemo(() => ({ activationConstraint: { distance: 6 }, locked: lockRef }), []);
  const touch = useMemo(() => ({ activationConstraint: { delay: 180, tolerance: 5 }, locked: lockRef }), []);
  const keyboard = useMemo(
    () => ({ coordinateGetter: sortableKeyboardCoordinates, keyboardCodes, locked: lockRef }),
    [keyboardCodes],
  );
  return useSensors(
    useSensor(LockableMouseSensor, mouse),
    useSensor(LockableTouchSensor, touch),
    useSensor(LockableKeyboardSensor, keyboard),
  );
}
