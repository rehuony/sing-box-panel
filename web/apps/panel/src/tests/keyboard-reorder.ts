import { vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { fireEvent, screen, waitFor } from '@testing-library/react';

/**
 * Moves a sortable entry from the keyboard: F2 on its grip, an arrow key per
 * step, down for a positive count, then F2 to drop it once the list
 * announces the expected position. jsdom lays nothing out, so the elements
 * `row` matches stack `height` apart while the entry moves.
 */
export async function keyboardReorder(grip: HTMLElement, { height = 52, position, row, steps }: {
  /** A selector for the sortable rows, such as `tr`. */
  row: string;
  steps: number;
  /** The 1-based position the list announces before the drop. */
  position: number;
  height?: number;
}) {
  const original = HTMLElement.prototype.getBoundingClientRect;
  const rect = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    if (this.matches(row)) {
      const index = Array.from(this.parentElement?.children ?? []).indexOf(this);
      return DOMRect.fromRect({ x: 0, y: index * height, width: 640, height });
    }
    if (this.classList.contains('cursor-grabbing')) return DOMRect.fromRect({ x: 0, y: 0, width: 640, height });
    return original.call(this);
  });
  try {
    const user = userEvent.setup();
    grip.focus();
    fireEvent.keyDown(grip, { key: 'F2', code: 'F2' });
    for (let step = 0; step < Math.abs(steps); step += 1) await user.keyboard(steps > 0 ? '[ArrowDown]' : '[ArrowUp]');
    await waitFor(() => screen.getByText(new RegExp(`^Position ${position} of \\d+\\.$`)));
    fireEvent.keyDown(grip, { key: 'F2', code: 'F2' });
  } finally {
    rect.mockRestore();
  }
}
