export const dashboard = {
  metric: {
    usageUnknown: 'Usage unknown',
    cpu: 'Host CPU',
    cpuDetail: '{{count}} cores · 1m load {{load}}',
    memory: 'Host memory',
    disk: 'Disk usage',
    transfer: 'Period traffic',
    label: 'Node figures',
    quota: 'Used {{used}} · quota {{total}}',
    countUnit: 'count',
    lastHour: 'Last hour',
    lastDay: 'Last 24 hours',
  },
  chart: {
    arrows: 'Use arrow keys to inspect samples.',
  },
  empty: {
    title: 'No samples in this range',
  },
  error: {
    metrics: 'Live metrics are unavailable',
    history: 'History is unavailable',
  },
  range: {
    label: 'Time range',
    option: {
      'live': 'Live',
      '1h': '1h',
      '6h': '6h',
      '24h': '24h',
      '7d': '7d',
      '30d': '30d',
      '90d': '90d',
    },
  },
  state: {
    liveLoading: 'Waiting for live samples…',
    liveStale: 'Live data delayed · last updated {{time}}',
    historyStale: 'History delayed · last updated {{time}}',
    historyLoading: 'Loading matching history…',
    runtimeLoading: 'Loading runtime history…',
    activityLoading: 'Loading recent activity…',
  },
  activity: {
    empty: 'No activity records.',

    title: 'Recent activity',
    viewAll: 'View all',
  },
  timeline: {
    ariaLabel: 'Observed runtime state across the selected range',
    state: {
      failed: 'failed',
      running: 'running',
      stopped: 'stopped',
      unknown: 'unknown',
    },
    title: 'Runtime timeline',
  },
  trend: {
    connections: 'Connections',
    description: 'Persisted samples; gaps remain gaps.',
    download: 'Download',
    memory: 'Memory',
    metricLabel: 'Trend metric',
    title: 'History',
    traffic: 'Traffic',
    upload: 'Upload',
  },
} as const;
