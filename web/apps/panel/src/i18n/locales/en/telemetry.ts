export const telemetry = {
  action: {
    failed: '{{action}} failed',
    executing: '{{action}} in progress…',
    timedOut: '{{action}} verification timed out',
    verified: '{{action}} verified',
    verifying: 'Verifying {{action}}…',
  },
  confirm: {
    cancel: 'Cancel',
    restart: {
      action: 'Restart sing-box',
      description: 'The current process will restart. Traffic may pause briefly.',
      title: 'Restart the sing-box runtime?',
    },
    stop: {
      action: 'Stop sing-box',
      description: 'Stop the currently running core.',
      title: 'Stop the sing-box runtime?',
    },
  },
  control: {
    label: 'Runtime controls',
    restart: 'Restart',
    start: 'Start',
    stop: 'Stop',
  },
  metric: {
    download: 'Download',
    upload: 'Upload',
    uptime: 'Uptime',
    version: 'Version',
  },
  moreActions: 'Runtime actions',
  runtime: {
    evidenceUnavailable: 'Runtime evidence unavailable',
    inspectionUnavailable: 'Runtime inspection unavailable',
    running: 'Running',
    runningDetail: 'Verified process',
    stale: 'Runtime evidence is stale',
    stopped: 'Stopped',
    stoppedDetail: 'Verified stopped state',
    unknown: '—',
  },
  startedAt: 'Started {{value}}',
  stream: {
    reconnecting: 'Live updates reconnecting…',
    detail: 'Live updates are interrupted. Keeping the last observation while reconnecting.',
  },
  unit: {
    day: 'd',
    hour: 'h',
    minute: 'm',
    second: 's',
  },
} as const;
