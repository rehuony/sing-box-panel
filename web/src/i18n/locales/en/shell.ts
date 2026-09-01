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
