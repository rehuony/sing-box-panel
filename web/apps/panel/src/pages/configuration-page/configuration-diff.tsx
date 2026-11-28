import { lazy, Suspense } from 'react';
import { useTranslation } from 'react-i18next';

import { LoadingState } from '@/components/loading-state';

// CodeMirror loads with the first difference shown, not with the configuration page.
const ConfigurationDiffView = lazy(() => import('./configuration-diff-view').then(module => ({ default: module.ConfigurationDiffView })));

/**
 * What replacing `original` with `next` changes, as one read-only document:
 * the next text with the removed lines shown where they were. Until the
 * editor has loaded, the source's loading state stays in its place.
 */
export function ConfigurationDiff({ next, original }: { next: string; original: string }) {
  const { t } = useTranslation();
  return (
    <Suspense fallback={<LoadingState fullScreen={false} label={t('configuration.copy.loading')} />}>
      <ConfigurationDiffView next={next} original={original} />
    </Suspense>
  );
}
