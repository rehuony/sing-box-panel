export const shell = {
  error: {
    badge: 'Context unavailable',
    contextUnavailable: 'Panel context is unavailable. The service may still be starting.',
    description: 'The panel service did not answer.',
    logout: 'Sign out failed; your current session is still active',
    retry: 'Try again',
  },
  loading: {
    description: 'Loading exact runtime identity and configuration evidence.',
    page: 'Loading workspace…',
    title: 'Reading panel context',
  },
  notSelected: 'Not selected',
  panelStatus: {
    loading: 'Checking panel status',
    online: 'Panel online',
    unavailable: 'Panel status unavailable',
  },
  skip: 'Skip to main content',
  version: 'v{{version}}',
} as const;

export const app = {
  title: 'Sing-Box Panel',
} as const;

export const nav = {
  panel: 'Panel settings',
  configuration: 'Configuration',
  cores: 'Versions',
  dashboard: 'Dashboard',
  observability: 'Runtime logs',
  subscriptions: 'Subscriptions',
} as const;

export const account = {
  signOut: 'Sign out',
  signingOut: 'Signing out…',
} as const;

export const sidebar = {
  mobileDescription: 'Displays the mobile navigation.',
  title: 'Navigation',
  toggle: 'Toggle navigation',
} as const;

export const theme = {
  cycle: 'Theme: {{current}}. Switch to {{next}}',
  preference: {
    dark: 'Dark',
    light: 'Light',
    system: 'System',
  },
} as const;

export const language = {
  english: 'English',
  label: 'Language',
  menu: 'Open language menu',
  simplifiedChinese: '简体中文',
} as const;
