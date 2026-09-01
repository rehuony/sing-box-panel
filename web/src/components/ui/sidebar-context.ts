import { createContext, use } from 'react';

import i18n from '@/i18n';

export interface SidebarContextValue {
  open: boolean;
  isMobile: boolean;
  openMobile: boolean;
  toggleSidebar: () => void;
  state: 'expanded' | 'collapsed';
  setOpen: (open: boolean) => void;
  setOpenMobile: (open: boolean) => void;
}

export const SidebarContext = createContext<SidebarContextValue | null>(null);

export function useSidebar(): SidebarContextValue {
  const context = use(SidebarContext);
  if (context === null) {
    throw new Error(i18n.t('common.providerRequired', {
      hook: 'useSidebar',
      provider: 'SidebarProvider',
    }));
  }

  return context;
}
