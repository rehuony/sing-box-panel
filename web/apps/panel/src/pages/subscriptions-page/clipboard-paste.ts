import { toast } from '@shared/components/ui/toast-manager';

import i18n from '@/i18n';

/** The keys that paste on this device. */
function pasteKeys(): string {
  return /Mac|iPhone|iPad/.test(navigator.userAgent) ? '⌘V' : 'Ctrl+V';
}

/**
 * Pastes the clipboard's text. Where the browser will not let a page read it,
 * as outside a secure context or when the reading is refused, the field takes
 * the focus so the keyboard's paste lands in it, and a note says which keys
 * paste; nothing is marked wrong, since nothing is.
 */
export function pasteFromClipboard(take: (text: string) => void, field: HTMLElement | null) {
  const reading = navigator.clipboard?.readText() ?? Promise.reject(new Error('clipboard unavailable'));
  reading.then(take, () => {
    field?.focus();
    toast.add({
      title: i18n.t('subscriptions.sources.content.clipboardUnavailable'),
      description: i18n.t('subscriptions.sources.content.pasteWithKeys', { keys: pasteKeys() }),
      type: 'info',
    });
  });
}
