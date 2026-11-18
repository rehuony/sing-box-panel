export const shell = {
  error: {
    logout: 'Sign out failed; your current session is still active',
    retry: 'Try again',
  },
  loading: {
    page: 'Loading workspace…',
  },
  bar: {
    label: 'Status line',
    more: 'More',
    next: 'Next node: {{name}}',
    previous: 'Previous node: {{name}}',
    windows: 'Windows',
  },
  readout: {
    label: '{{online}} of {{total}} nodes online',
    online: '{{online}}/{{total}} online',
    stale: 'The fleet stream is down; these values may be out of date',
  },
  scope: {
    fleet: 'All nodes',
  },
  selection: {
    clear: 'Clear selection',
    count: '{{count}} selected',
    selectAll: 'Select all',
  },
  update: {
    available: 'The panel can update to {{version}}',
    restart: 'restart to update',
    running: 'updating',
  },
  window: '{{number}}: {{name}}',
  skip: 'Skip to main content',
} as const;

export const nodes = {
  loadFailed: 'Nodes could not be loaded.',
  notSelected: 'Add a node to manage its cores and configuration.',
  state: {
    offline: 'Offline',
    online: 'Online',
  },
} as const;

export const nav = {
  logs: 'Logs',
  nodes: 'Nodes',
  settings: 'Settings',
  subscriptions: 'Subscriptions',
} as const;

export const account = {
  signOut: 'Sign out',
  signingOut: 'Signing out…',
} as const;

export const theme = {
  label: 'Theme',
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

export const command = {
  batch: {
    actions: 'Batch actions',
    run_one: '{{action}} {{count}} node',
    run_other: '{{action}} {{count}} nodes',
    title_one: '{{action}} {{count}} node?',
    title_other: '{{action}} {{count}} nodes?',
    versions: {
      choose: 'Choose a version',
      label: 'Core version to install',
      loading: 'Loading the catalog…',
      none: 'The catalog has no versions; refresh it in a node\'s core versions.',
    },
    action: {
      install_core: 'Install core',
      restart: 'Restart',
      start: 'Start',
      stop: 'Stop',
      update: 'Update',
    },
    busy: 'a batch is still running; wait for it to finish',
    cancelled: 'cancelled',
    confirm_one: '{{action}} {{count}} node: {{names}}',
    confirm_other: '{{action}} {{count}} nodes: {{names}}',
    failed: 'failed: {{reason}}',
    incomplete: 'the batch stopped reporting before its summary; check each node\'s state',
    interrupts: {
      install_core: 'Installing a core does not change what runs; select it on each node to use it.',
      restart: 'Restarting a core briefly drops its connections.',
      stop: 'Stopping a core drops its connections until it starts again.',
      update: 'Each node restarts with its core after updating, one node at a time; the first failure stops the rest.',
    },
    more: 'and {{count}} more',
    noMatch: 'no nodes match {{target}}',
    ok: 'done',
    running_one: '{{action}} on {{count}} node…',
    running_other: '{{action}} on {{count}} nodes…',
    skipped: 'skipped after an earlier failure',
    summary: '{{succeeded}} succeeded · {{failed}} failed · {{skipped}} skipped',
    toast: '{{action}} finished',
    toastIncomplete: '{{action}} did not report its outcome',
  },
  confirm: 'Confirm with y',
  dismiss: 'Close output',
  error: {
    ambiguous: 'several nodes are named {{name}}; use its ID',
    noNode: 'no node named {{name}}',
    noWindow: 'no window {{position}}',
    notFound: 'command not found: {{command}}',
    unavailable: '{{command}} is not available here',
  },
  help: {
    clear: 'Clear the output',
    exit: 'Sign out',
    filter: 'Filter nodes: text, state:online|offline or tag:<tag>; empty clears',
    goto: 'Open a node, at a window if given',
    help: 'List the commands',
    logout: 'Sign out',
    number: 'Open the window with this number',
    open: 'Open nodes, subscriptions, logs or settings',
    restart: 'Restart the cores of nodes: a name, a glob such as hk-*, tag:<tag> or all',
    start: 'Start the cores of nodes: a name, a glob such as hk-*, tag:<tag> or all',
    stop: 'Stop the cores of nodes: a name, a glob such as hk-*, tag:<tag> or all',
    update: 'Update nodes to the panel\'s release, one at a time',
  },
  input: 'Command',
  label: 'Command line',
  output: 'Output',
  shortcut: 'Command line ({{keys}} or :)',
  usage: {
    filter: 'usage: filter [text | state:online|offline | tag:<tag>]',
    goto: 'usage: goto <node> [overview|cores|configuration|logs|settings]',
    open: 'usage: open nodes|subscriptions|logs|settings',
    restart: 'usage: restart <node | glob | tag:<tag> | all>',
    start: 'usage: start <node | glob | tag:<tag> | all>',
    stop: 'usage: stop <node | glob | tag:<tag> | all>',
    update: 'usage: update <node | glob | tag:<tag> | all>',
  },
} as const;

export const terminal = {
  completions: 'Completions',
  log: {
    label: 'Command output',
  },
} as const;
