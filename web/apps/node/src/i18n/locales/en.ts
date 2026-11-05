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
  connection: {
    lastError: 'Last error',
    panel: 'Panel {{address}}',
    retryAt: 'Next attempt',
    since: 'Since {{time}}',
    state: {
      connected: 'Connected',
      connecting: 'Connecting',
      disconnected: 'Disconnected',
      unenrolled: 'Not enrolled',
    },
    title: 'Connection',
  },
  console: {
    checking: 'Checking the console session…',
    disabled: {
      description: 'The console is disabled on this node. Enable it with sing-box-node console enable.',
      title: 'Console disabled',
    },
    loading: 'Loading the node’s status…',
    signOut: 'Sign out',
    stale: 'The node did not answer; showing the last status.',
    title: 'Node console',
    unavailable: {
      description: 'The node did not answer. Check that sing-box-node is running.',
      title: 'Console unavailable',
    },
  },
  danger: {
    reenroll: 'Re-enroll',
    reenrollConfirm: 'The node is enrolled with {{address}}. Enrolling with a new code replaces that enrollment and ends the current session; the runtime keeps its applied configuration.',
    reenrollDescription: 'Replace this node’s enrollment with a new code from a panel.',
    reenrollTitle: 'Re-enroll this node',
    title: 'Enrollment',
    unenroll: 'Unenroll',
    unenrollConfirm: 'The node ends its session with {{address}} and forgets the enrollment. The panel keeps the node, shown offline, until it is removed there; the runtime keeps its applied configuration.',
    unenrollDescription: 'End the session with the panel and remove this node’s enrollment.',
    unenrollFailed: 'The node could not unenroll. Try again.',
    unenrollTitle: 'Unenroll from {{address}}?',
  },
  details: {
    hostname: 'Hostname',
    node: 'This node',
    nodeId: 'Node ID',
    panelAddress: 'Panel address',
    platform: 'Platform',
    protocol: 'Protocol',
    title: 'Panel and node',
    trust: {
      label: 'Trust',
      pin: 'Pinned certificate',
      system: 'System certificate authorities',
    },
    version: 'Version',
  },
  enroll: {
    code: 'Enrollment code',
    description: 'Add this node in the panel, then paste the one-time code it shows. Codes work once and expire after an hour.',
    error: {
      empty: 'Paste the enrollment code from the panel.',
      failed: 'The node could not enroll. Try again.',
      invalid: 'This is not an enrollment code. Copy it again from the panel.',
      rejected: 'The panel did not accept this code. It may have been used or expired; issue a new one in the panel.',
      unreachable: 'The panel named in the code did not answer. Check that it is reachable from this host, then try again.',
    },
    placeholder: 'Paste the code from the panel',
    submit: 'Enroll',
    title: 'Enroll this node',
  },
  language: {
    english: 'English',
    label: 'Language',
    menu: 'Open language menu',
    simplifiedChinese: '简体中文',
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
  theme: {
    cycle: 'Theme: {{current}}. Switch to {{next}}',
    preference: {
      dark: 'Dark',
      light: 'Light',
      system: 'System',
    },
  },
};

export type NodeTranslation = typeof en;
