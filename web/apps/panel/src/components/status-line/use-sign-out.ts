import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from '@shared/components/ui/toast-manager';

import { useAuthSession } from '@/stores/auth-session.store';
import { useUnsavedChangesContext } from '@/stores/unsaved-changes.store';

/** Signs out after any unsaved changes are kept or discarded; a failure keeps the session and says so. */
export function useSignOut() {
  const { t } = useTranslation();
  const { logout } = useAuthSession();
  const { confirmNavigation } = useUnsavedChangesContext();
  const [signingOut, setSigningOut] = useState(false);

  function signOut() {
    if (signingOut) return;
    confirmNavigation(() => {
      setSigningOut(true);
      logout()
        .catch(() => toast.add({ title: t('shell.error.logout'), type: 'error' }))
        .finally(() => setSigningOut(false));
    });
  }

  return { signOut, signingOut };
}
