export const pageLoaders = {
  '/': () => import('@/pages/dashboard-page/dashboard-page').then(page => ({ default: page.DashboardPage })),
  '/configuration': () => import('@/pages/configuration-page/configuration-page').then(page => ({ default: page.ConfigurationPage })),
  '/cores': () => import('@/pages/cores-page/cores-page').then(page => ({ default: page.CoresPage })),
  '/panel': () => import('@/pages/panel-settings-page/panel-settings-page').then(page => ({ default: page.PanelSettingsPage })),
  '/subscriptions': () => import('@/pages/subscriptions-page/subscriptions-page').then(page => ({ default: page.SubscriptionsPage })),
  '/observability': () => import('@/pages/observability-page/observability-page').then(page => ({ default: page.ObservabilityPage })),
};

type PagePath = keyof typeof pageLoaders;
type PageModule = Awaited<ReturnType<typeof pageLoaders[PagePath]>>;

const preloads = new Map<PagePath, Promise<PageModule>>();
let preloadFailed = false;

export function preloadPage(path: PagePath) {
  if (preloadFailed || preloads.has(path)) return;
  const pending = pageLoaders[path]();
  preloads.set(path, pending);
  // Failed CSS can remain in Vite's shared preload registry. Do not retry imports
  // in this document or interrupt the current page because of a hover failure.
  void pending.catch(() => {
    preloadFailed = true;
  });
}

function reloadForNavigation(): Promise<never> {
  window.location.reload();
  return new Promise(() => {});
}

export async function loadPage(path: PagePath): Promise<PageModule> {
  if (preloadFailed) return reloadForNavigation();
  const pending = preloads.get(path);
  try {
    const page = await (pending ?? pageLoaders[path]());
    return preloadFailed ? reloadForNavigation() : page;
  } catch (error) {
    // Navigation may have started while the speculative request was pending.
    if (pending || preloadFailed) return reloadForNavigation();
    throw error;
  }
}
