import { cores } from './cores';
import { login } from './login';
import { tasks } from './tasks';
import { channels } from './channels';
import { notFound } from './not-found';
import { dashboard } from './dashboard';
import { telemetry } from './telemetry';
import { pagination } from './pagination';
import { bootstrap, common } from './common';
import { productLogs } from './product-logs';
import { configuration } from './configuration';
import { observability } from './observability';
import { subscriptions } from './subscriptions';
import { panelSettings } from './panel-settings';
import { account, app, language, nav, shell, sidebar, theme } from './shell';

export const en = {
  productLogs,
  account,
  channels,
  app,
  bootstrap,
  common,
  configuration,
  cores,
  dashboard,
  language,
  theme,
  nav,
  notFound,
  observability,
  login,
  pagination,
  panelSettings,
  sidebar,
  shell,
  subscriptions,
  tasks,
  telemetry,
} as const;
