import { cores } from './cores';
import { login } from './login';
import { channels } from './channels';
import { notFound } from './not-found';
import { dashboard } from './dashboard';
import { telemetry } from './telemetry';
import { filesystem } from './filesystem';
import { pagination } from './pagination';
import { bootstrap, common } from './common';
import { productLogs } from './product-logs';
import { configuration } from './configuration';
import { subscriptions } from './subscriptions';
import { panelSettings } from './panel-settings';
import { enrollment, fleet, node, tags } from './fleet';
import { account, command, language, nav, nodes, shell, terminal, theme } from './shell';

export const en = {
  enrollment,
  fleet,
  node,
  productLogs,
  filesystem,
  account,
  channels,
  bootstrap,
  command,
  common,
  configuration,
  cores,
  dashboard,
  language,
  theme,
  nav,
  nodes,
  notFound,
  login,
  pagination,
  panelSettings,
  shell,
  terminal,
  subscriptions,
  tags,
  telemetry,
} as const;
