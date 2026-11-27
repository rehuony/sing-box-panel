import { useEffect, useId, useState } from 'react';
import { toast } from '@shared/components/ui/toast-manager';

import { messageLine } from '@/utils/message-line';

interface Problem<Field extends string> {
  /** The field the problem is in, marked until it is edited. */
  field?: Field;
  /** Why, under the title, such as a refused save's reason. */
  description?: string;
  type?: 'error' | 'warning';
}

/**
 * What keeps a dialog from going on, said in its one toast so the dialog
 * itself never moves: the next problem replaces the toast and an edit closes
 * it, and so does closing the dialog. The field a problem is in stays marked
 * until then; the dialog focuses it.
 *
 * @param open Whether the dialog is open, for one that stays mounted while
 * closed; one that unmounts closes its toast as it goes.
 */
export function useDialogProblem<Field extends string = never>(open = true) {
  const toastId = `${useId()}-problem`;
  const [field, setField] = useState<Field | null>(null);
  useEffect(() => () => toast.close(toastId), [open, toastId]);
  return {
    report(title: string, { description, field: at, type = 'error' }: Problem<Field> = {}) {
      setField(at ?? null);
      toast.add({ id: toastId, title: messageLine(title), description: description && messageLine(description), type });
    },
    /** Clears the mark and closes the toast, as an edit does. */
    clear() {
      setField(null);
      toast.close(toastId);
    },
    /** The `aria-invalid` value of a field: true while a problem is in it. */
    invalid: (name: Field) => field === name || undefined,
  };
}
