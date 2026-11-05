import { use } from 'react';
import { PanelLeft } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { SidebarContext } from '@shared/components/ui/sidebar-context';

/** Opens the navigation rail where it collapses into a sheet; each view's first row carries it. */
export function ShellNavigationTrigger() {
  const { t } = useTranslation();
  const sidebar = use(SidebarContext);
  // A view rendered outside the shell has no rail to open.
  if (sidebar === null) return null;
  return (
    <Button
      aria-label={t('shell.openNavigation')}
      className='shrink-0 md:hidden'
      onClick={() => sidebar.setOpenMobile(true)}
      size='icon-sm'
      variant='ghost'
    >
      <PanelLeft aria-hidden='true' />
    </Button>
  );
}
