export const shell = {
  error: {
    logout: 'Sign out failed; your current session is still active',
    page: 'The panel could not be displayed.',
    reload: 'Reload',
    retry: 'Try again',
    view: 'This view could not be loaded.',
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
  restart: {
    action: 'Restart panel',
    confirm: 'Restart panel',
    description: 'The panel will be unavailable for a few seconds. Nodes and their cores keep running; subscriptions cannot update meanwhile.',
    done: 'Panel restarted',
    running: 'Restarting panel…',
    timeout: 'The panel did not come back',
    title: 'Restart panel',
  },
  scope: {
    fleet: 'All nodes',
  },
  selection: {
    count: '{{count}} selected',
    exit: 'Exit selection',
    selectAll: 'Select all',
    selectNone: 'Select none',
  },
  update: {
    available: 'The panel can update to {{version}}',
    restart: 'restart to update',
    running: 'updating',
  },
  window: '{{number}}: {{name}}',
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
    title_one: '{{action}} {{count}} node?',
    title_other: '{{action}} {{count}} nodes?',
    titleNode: '{{action}} {{name}}?',
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
    doing: {
      install_core: 'installing',
      restart: 'restarting',
      start: 'starting',
      stop: 'stopping',
      update: 'updating',
    },
    done: {
      install_core: 'installed {{version}}',
      restart: 'restarted',
      start: 'started',
      stop: 'stopped',
      update: 'updated',
    },
    failedCount: '{{count}} failed',
    heading_one: '{{action}} {{count}} node',
    heading_other: '{{action}} {{count}} nodes',
    incomplete: 'the batch stopped reporting before its summary; check each node\'s state',
    interrupts: {
      install_core: 'Installs only; running cores keep their version.',
      restart: 'Proxied connections drop briefly during the restart.',
      stop: 'Proxied connections stop until started again.',
      update: 'The node and its core restart; its proxy drops briefly.',
      updateMany: 'One node at a time; each proxy drops briefly as its node restarts.',
    },
    left_one: '{{count}} selected node does not fit and is left out',
    left_other: '{{count}} selected nodes do not fit and are left out',
    lost: 'connection lost',
    lostDetail: 'results continue once the panel answers',
    more: 'and {{count}} more',
    noMatch: 'no nodes match {{target}}',
    noneFit: '{{action}} fits none of these nodes',
    skipped: 'skipped after an earlier failure',
    skippedCount: '{{count}} skipped',
    succeeded: '{{count}} succeeded',
    summary: '{{succeeded}} succeeded · {{failed}} failed · {{skipped}} skipped',
    took: 'took {{time}}',
    toast: '{{action}} finished',
    toastIncomplete: '{{action}} did not report its outcome',
    unfit: {
      empty: 'Select nodes first',
      install_core: 'Every selected node is offline',
      restart: 'No selected node has a core to restart',
      start: 'No selected node has a stopped core',
      stop: 'No selected node has a running core',
      update: 'No selected node has an update to run',
    },
    unreported: '{{count}} unreported',
    waiting: '{{count}} waiting',
  },
  arguments: {
    filter: '[filter]',
    goto: '<node> [window]',
    open: '<window>',
    restart: '<target>',
    start: '<target>',
    stop: '<target>',
    update: '<target>',
  },
  completion: {
    all: 'Every node',
    tag: 'Nodes tagged {{tag}}',
  },
  confirm: 'Confirm with y',
  confirmScope: 'Confirm',
  error: {
    ambiguous: 'several nodes are named {{name}}; use its ID',
    noNode: 'no node named {{name}}',
    noWindow: 'no window {{position}}',
    notFound: 'command not found: {{command}}',
  },
  help: {
    clear: 'Clear the output',
    exit: 'Sign out',
    filter: 'Filter nodes, empty clears',
    goto: 'Open a node',
    help: 'List commands',
    number: 'Open the window with this number',
    open: 'Open a panel window',
    reboot: 'Restart the panel',
    restart: 'Restart nodes\' cores',
    start: 'Start nodes\' cores',
    stop: 'Stop nodes\' cores',
    update: 'Update nodes to the panel\'s version',
  },
  input: 'Command',
  label: 'Command line',
  reboot: {
    busy: 'The panel is restarting',
    confirm: 'Restart the panel? It will be unavailable for a few seconds; nodes and cores keep running.',
    done: 'Panel restarted',
    running: 'Restarting panel…',
  },
  shortcut: 'Command line ({{keys}} or :)',
  usage: {
    filter: 'usage: filter [text | state:online|offline | tag:<tag>]',
    goto: 'usage: goto <node> [overview|cores|configuration|logs|settings]',
    open: 'usage: open nodes|subscriptions|logs|settings',
    reboot: 'usage: reboot',
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
