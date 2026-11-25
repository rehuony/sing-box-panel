export const dashboard = {
  metric: {
    usageUnknown: '用量未知',
    cpu: '主机 CPU',
    cpuDetail: '{{count}} 核 · 1 分钟负载 {{load}}',
    memory: '主机内存',
    disk: '磁盘使用',
    transfer: '周期流量',
    label: '节点概况',
    quota: '已用 {{used}} · 额度 {{total}}',
    countUnit: '个',
    lastDay: '最近 24 小时',
  },
  chart: {
    arrows: '使用左右方向键查看采样。',
  },
  error: {
    metrics: '实时指标不可用',
    history: '历史指标不可用',
  },
  range: {
    label: '时间范围',
    option: {
      'live': '实时',
      '1h': '1 小时',
      '6h': '6 小时',
      '24h': '24 小时',
      '7d': '7 天',
      '30d': '30 天',
      '90d': '90 天',
    },
  },
  state: {
    liveLoading: '正在等待实时采样…',
    liveStale: '实时数据延迟 · 上次更新 {{time}}',
    historyStale: '历史数据延迟 · 上次更新 {{time}}',
    historyLoading: '正在加载匹配的历史数据…',
    runtimeLoading: '正在加载运行历史…',
    activityLoading: '正在加载最近活动…',
  },
  activity: {
    empty: '暂无活动记录',

    title: '最近活动',
    viewAll: '查看全部',
  },
  timeline: {
    ariaLabel: '所选时间范围内已观测的运行状态',
    state: {
      failed: '异常',
      running: '运行中',
      stopped: '已停止',
      unknown: '未知',
    },
    now: '现在',
    title: '运行时间线',
  },
  trend: {
    connections: '连接数',
    description: '展示持久化样本；数据缺口保持为空。',
    download: '下行',
    memory: '内存',
    metricLabel: '趋势指标',
    title: '流量与连接',
    traffic: '流量',
    upload: '上行',
  },
} as const;
