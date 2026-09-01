import { app } from './app';
import { nav } from './nav';
import { cores } from './cores';
import { login } from './login';
import { shell } from './shell';
import { tasks } from './tasks';
import { theme } from './theme';
import { common } from './common';
import { account } from './account';
import { sidebar } from './sidebar';
import { language } from './language';
import { notFound } from './not-found';
import { bootstrap } from './bootstrap';
import { dashboard } from './dashboard';
import { telemetry } from './telemetry';
import { pagination } from './pagination';
import { configuration } from './configuration';
import { observability } from './observability';
import { subscriptions } from './subscriptions';

export const zhCN = {
  account,
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
  sidebar,
  shell,
  subscriptions,
  tasks,
  telemetry,
} as const;
