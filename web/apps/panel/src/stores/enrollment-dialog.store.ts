import { create } from 'zustand';

import type { NodeEnrollment } from '@/api/api-client';

/** A code to wait on: the node it enrolls, and its enrollment time before the code, which a new node has none of. */
export interface PendingEnrollment {
  enrollment: NodeEnrollment;
  enrolledBefore: string | null;
}

interface EnrollmentDialogState {
  open: boolean;
  /** Opens the dialog to add a node. */
  add: () => void;
  close: () => void;
  /** Ends the signed-in session's dialog. */
  reset: () => void;
  /** The code issued for an enrolled node, which the dialog opens waiting on; null while it adds a node. */
  reenrollment: PendingEnrollment | null;
  /** Opens the dialog waiting on a code issued for an enrolled node. */
  reenroll: (pending: PendingEnrollment) => void;
}

/**
 * The one enrollment dialog of the shell, outside every view, so a view that
 * changes or remounts while a host enrolls never takes the dialog with it.
 */
export const useEnrollmentDialog = create<EnrollmentDialogState>()(set => ({
  add: () => set({ open: true, reenrollment: null }),
  close: () => set({ open: false }),
  open: false,
  reenroll: reenrollment => set({ open: true, reenrollment }),
  reenrollment: null,
  reset: () => set({ open: false, reenrollment: null }),
}));
