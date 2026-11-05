export const en = {
  bootstrap: {
    rootMissing: 'The application root element is missing.',
  },
  common: {
    cancel: 'Cancel',
    close: 'Close',
    providerRequired: '{{hook}} must be used within {{provider}}.',
    retry: 'Try again',
    unavailable: '—',
  },
  console: {
    checking: 'Checking the console session…',
    disabled: {
      description: 'The console is disabled on this node. Enable it with sing-box-node console enable.',
      title: 'Console disabled',
    },
    title: 'Node console',
    unavailable: {
      description: 'The node did not answer. Check that sing-box-node is running.',
      title: 'Console unavailable',
    },
  },
  login: {
    error: {
      empty: 'Enter the console password.',
      rateLimited: 'Too many attempts. Wait a minute and try again.',
      unauthorized: 'The password is incorrect.',
      unreachable: 'The node did not answer. Try again.',
    },
    password: 'Console password',
    submit: 'Sign in',
    title: 'Sign in to the node console',
  },
  status: {
    connection: {
      lastError: 'Last error',
      retryAt: 'Next attempt',
      since: 'Since',
      stateLabel: 'State',
      state: {
        connected: 'Connected',
        connecting: 'Connecting',
        disconnected: 'Disconnected',
        unenrolled: 'Not enrolled',
      },
      title: 'Connection',
    },
    node: {
      platform: 'Platform',
      protocol: 'Protocol',
      title: 'Node',
      version: 'Version',
    },
    panel: {
      endpoint: 'Panel address',
      nodeId: 'Node ID',
      title: 'Panel',
      trust: {
        label: 'Trust',
        pin: 'Pinned certificate',
        system: 'System certificate authorities',
      },
    },
    signOut: 'Sign out',
    stale: 'The node did not answer; showing the last status.',
    unenrolled: {
      description: 'Add the node in the panel, then run the command it shows on this host: sing-box-node enroll --code <code>.',
      title: 'Not enrolled',
    },
  },
};

export type NodeTranslation = typeof en;
