export const shell = {
  error: {
    logout: '退出失败，当前会话仍然有效',
    page: '无法显示面板。',
    reload: '重新加载',
    retry: '重试',
    view: '无法加载此视图。',
  },
  loading: {
    page: '正在载入工作台…',
  },
  bar: {
    label: '状态栏',
    more: '更多',
    next: '下一个节点：{{name}}',
    previous: '上一个节点：{{name}}',
    windows: '窗口',
  },
  readout: {
    label: '{{total}} 个节点中 {{online}} 个在线',
    online: '{{online}}/{{total}} 在线',
    stale: '节点状态流已中断，这些数值可能已过时',
  },
  restart: {
    action: '重启面板',
    confirm: '重启面板',
    description: '面板会中断几秒钟。节点和核心继续运行，订阅在此期间无法更新。',
    done: '面板已重启',
    running: '面板正在重启…',
    timeout: '面板没有恢复响应',
    title: '重启面板',
  },
  scope: {
    fleet: '全部节点',
  },
  selection: {
    count: '已选 {{count}} 个',
    exit: '退出多选',
    selectAll: '全选',
    selectNone: '全不选',
  },
  update: {
    available: '面板可更新到 {{version}}',
    restart: '重启以完成更新',
    running: '正在更新',
  },
  window: '{{number}}：{{name}}',
} as const;

export const nodes = {
  loadFailed: '无法加载节点。',
  notSelected: '请先添加节点，再管理其内核与配置。',
  state: {
    offline: '离线',
    online: '在线',
  },
} as const;

export const nav = {
  logs: '日志',
  nodes: '节点',
  settings: '设置',
  subscriptions: '订阅',
} as const;

export const account = {
  signOut: '退出登录',
  signingOut: '正在退出…',
} as const;

export const theme = {
  label: '主题',
  cycle: '当前主题：{{current}}。切换为{{next}}',
  preference: {
    dark: '深色',
    light: '亮色',
    system: '系统',
  },
} as const;

export const language = {
  english: 'English',
  label: '语言',
  menu: '打开语言菜单',
  simplifiedChinese: '简体中文',
} as const;

export const command = {
  batch: {
    actions: '批量操作',
    title_one: '{{action}} {{count}} 个节点？',
    title_other: '{{action}} {{count}} 个节点？',
    titleNode: '{{action}} {{name}}？',
    versions: {
      choose: '选择版本',
      label: '要安装的核心版本',
      loading: '正在载入版本目录…',
      none: '版本目录为空；请在节点的核心版本中刷新。',
    },
    action: {
      install_core: '安装核心',
      restart: '重启',
      start: '启动',
      stop: '停止',
      update: '更新',
    },
    busy: '仍有批量操作在运行，请等待其完成',
    cancelled: '已取消',
    confirm_one: '{{action}} {{count}} 个节点：{{names}}',
    confirm_other: '{{action}} {{count}} 个节点：{{names}}',
    doing: {
      install_core: '安装中',
      restart: '重启中',
      start: '启动中',
      stop: '停止中',
      update: '更新中',
    },
    done: {
      install_core: '已安装 {{version}}',
      restart: '已重启',
      start: '已启动',
      stop: '已停止',
      update: '已更新',
    },
    failedCount: '失败 {{count}}',
    heading_one: '{{action}} {{count}} 个节点',
    heading_other: '{{action}} {{count}} 个节点',
    incomplete: '批量操作在给出汇总前中断了报告；请查看各节点的状态',
    interrupts: {
      install_core: '只安装，正在运行的核心不会切换。',
      restart: '重启时代理连接短暂中断。',
      stop: '代理连接会中断，直到再次启动。',
      update: '节点与核心将重启，代理短暂中断。',
      updateMany: '逐个更新，节点与核心重启时代理短暂中断。',
    },
    left_one: '跳过 {{count}} 个不适用的节点',
    left_other: '跳过 {{count}} 个不适用的节点',
    lost: '连接中断',
    lostDetail: '恢复后继续显示结果',
    more: '等 {{count}} 个',
    noMatch: '没有匹配 {{target}} 的节点',
    noneFit: '没有可{{action}}的节点',
    skipped: '因先前失败而跳过',
    skippedCount: '跳过 {{count}}',
    succeeded: '成功 {{count}}',
    summary: '成功 {{succeeded}} · 失败 {{failed}} · 跳过 {{skipped}}',
    took: '用时 {{time}}',
    toast: '{{action}}已完成',
    toastIncomplete: '{{action}}未报告结果',
    unfit: {
      empty: '先选择节点',
      install_core: '所选节点都离线',
      restart: '所选节点没有可重启的核心',
      start: '所选节点没有已停止的核心',
      stop: '所选节点没有运行中的核心',
      update: '所选节点没有可用的更新',
    },
    unreported: '未报告 {{count}} 个',
    waiting: '等待 {{count}} 个',
  },
  arguments: {
    filter: '[条件]',
    goto: '<节点> [窗口]',
    open: '<窗口>',
    restart: '<目标>',
    start: '<目标>',
    stop: '<目标>',
    update: '<目标>',
  },
  completion: {
    all: '全部节点',
    tag: '带 {{tag}} 标签的节点',
  },
  confirm: '输入 y 确认',
  confirmScope: '确认',
  error: {
    ambiguous: '有多个节点名为 {{name}}，请使用节点 ID',
    noNode: '没有名为 {{name}} 的节点',
    noWindow: '没有编号为 {{position}} 的窗口',
    notFound: '未找到命令：{{command}}',
  },
  help: {
    clear: '清空输出',
    exit: '退出登录',
    filter: '筛选节点，留空清除',
    goto: '打开节点',
    help: '列出命令',
    number: '打开对应编号的窗口',
    open: '打开面板窗口',
    reboot: '重启面板',
    restart: '重启节点的核心',
    start: '启动节点的核心',
    stop: '停止节点的核心',
    update: '将节点更新到面板的版本',
  },
  input: '命令',
  label: '命令行',
  reboot: {
    busy: '面板正在重启',
    confirm: '重启面板？面板会中断几秒钟，节点和核心继续运行。',
    done: '面板已重启',
    running: '正在重启面板…',
  },
  shortcut: '命令行（{{keys}} 或 :）',
  usage: {
    filter: '用法：filter [文本 | state:online|offline | tag:<标签>]',
    goto: '用法：goto <节点> [overview|cores|configuration|logs|settings]',
    open: '用法：open nodes|subscriptions|logs|settings',
    reboot: '用法：reboot',
    restart: '用法：restart <节点 | 通配 | tag:<标签> | all>',
    start: '用法：start <节点 | 通配 | tag:<标签> | all>',
    stop: '用法：stop <节点 | 通配 | tag:<标签> | all>',
    update: '用法：update <节点 | 通配 | tag:<标签> | all>',
  },
} as const;

export const terminal = {
  completions: '可补全项',
  log: {
    label: '命令输出',
  },
} as const;
