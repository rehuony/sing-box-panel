import { useEffect, useRef } from 'react';
import { useQuery } from '@tanstack/react-query';

import type { SystemUpdate } from '@/api/api-client';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';

/** States in which the panel's update is under way and its status worth following. */
const following = new Set<SystemUpdate['state']>(['updating', 'staged', 'restarting']);
/** The version this tab last reloaded for, so a reload happens once per version. */
const RELOADED_KEY = 'sing-box-panel.reloaded-for';

/** The panel's update status, read again every two seconds while an update is under way. */
export function usePanelUpdate() {
  const client = useApiClient();
  return useQuery({
    ...queries.systemUpdate(client),
    // A restarting panel answers nothing for a moment; keep asking until it runs again.
    refetchInterval: query => following.has(query.state.data?.state ?? 'idle') ? 2000 : false,
  });
}

/** Reloads the page once when the panel starts reporting another version, so the new interface replaces the old. */
export function useReloadOnPanelVersion() {
  const version = usePanelUpdate().data?.current_version;
  const firstRef = useRef<string | null>(null);
  useEffect(() => {
    if (version === undefined) return;
    if (firstRef.current === null) {
      firstRef.current = version;
      return;
    }
    if (version === firstRef.current) return;
    try {
      if (window.sessionStorage.getItem(RELOADED_KEY) === version) return;
      window.sessionStorage.setItem(RELOADED_KEY, version);
    } catch {
      // Without storage the reload still happens; the next page reads the new version first.
    }
    window.location.reload();
  }, [version]);
}
