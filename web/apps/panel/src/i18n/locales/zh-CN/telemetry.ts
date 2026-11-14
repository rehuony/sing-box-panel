export const telemetry = {
  action: {
    failed: '{{action}}失败',
    executing: '正在提交{{action}}…',
    timedOut: '{{action}}验证超时',
    verified: '{{action}}已验证',
    verifying: '正在验证{{action}}…',
  },
  confirm: {
    cancel: '取消',
    restart: {
      action: '重启 sing-box',
      description: '操作将更改内核运行状态。',
      title: '要重启 sing-box 吗？',
    },
    stop: {
      action: '停止 sing-box',
      description: '操作将更改内核运行状态。',
      title: '要停止 sing-box 吗？',
    },
  },
  control: {
    label: '运行控制',
    restart: '重启',
    start: '启动',
    stop: '停止',
  },
  metric: {
    connections: '连接',
    download: '下行',
    upload: '上行',
    uptime: '运行时长',
    version: '版本',
  },
  moreActions: '运行操作',
  runtime: {
    evidenceUnavailable: '运行证据不可用',
    inspectionUnavailable: '无法检查运行状态',
    running: '运行中',
    runningDetail: '已验证进程',
    stale: '运行证据已过期',
    stopped: '已停止',
    stoppedDetail: '已验证停止状态',
    unknown: '—',
  },
  startedAt: '启动于 {{value}}',
  stream: {
    reconnecting: '实时数据正在重连…',
    detail: '实时数据暂时中断，正在保留上次观测并重新连接。',
  },
  unit: {
    day: '天',
    hour: '小时',
    minute: '分钟',
    second: '秒',
  },
} as const;
