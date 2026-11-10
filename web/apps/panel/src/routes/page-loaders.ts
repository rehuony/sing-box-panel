export const pageLoaders = {
  'overview': () => import('@/pages/overview-page/overview-page').then(page => ({ default: page.OverviewPage })),
  'nodes': () => import('@/pages/fleet-page/fleet-page').then(page => ({ default: page.FleetPage })),
  'node-overview': () => import('@/pages/node-overview-page/node-overview-page').then(page => ({ default: page.NodeOverviewPage })),
  'node-cores': () => import('@/pages/cores-page/cores-page').then(page => ({ default: page.CoresPage })),
  'node-configuration': () => import('@/pages/configuration-page/configuration-page').then(page => ({ default: page.ConfigurationPage })),
  'node-logs': () => import('@/pages/node-logs-page/node-logs-page').then(page => ({ default: page.NodeLogsPage })),
  'node-settings': () => import('@/pages/node-settings-page/node-settings-page').then(page => ({ default: page.NodeSettingsPage })),
  'subscriptions': () => import('@/pages/subscriptions-page/subscriptions-page').then(page => ({ default: page.SubscriptionsPage })),
  'logs': () => import('@/pages/panel-logs-page/panel-logs-page').then(page => ({ default: page.PanelLogsPage })),
  'settings': () => import('@/pages/panel-settings-page/panel-settings-page').then(page => ({ default: page.PanelSettingsPage })),
};

export type PagePath = keyof typeof pageLoaders;
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
