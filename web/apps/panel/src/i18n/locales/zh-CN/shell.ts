export const shell = {
  error: {
    logout: '退出失败，当前会话仍然有效',
    retry: '重试',
  },
  loading: {
    page: '正在载入工作台…',
  },
  bar: {
    label: '状态栏',
    more: '更多',
    next: '下一个节点：{{name}}',
    previous: '上一个节点：{{name}}',
    stale: '已过时',
    windows: '窗口',
  },
  readout: {
    label: '{{total}} 个节点中 {{online}} 个在线',
    online: '{{online}}/{{total}} 在线',
    stale: '节点状态流已中断，这些数值可能已过时',
  },
  scope: {
    fleet: '全部节点',
    label: '范围',
  },
  selection: {
    clear: '清除选择',
    count: '已选 {{count}} 个',
    selectAll: '全选',
  },
  update: {
    available: '面板可更新到 {{version}}',
    restart: '重启以完成更新',
    running: '正在更新',
  },
  window: '{{number}}：{{name}}',
  skip: '跳到主要内容',
} as const;

export const nodes = {
  loadFailed: '无法加载节点。',
  notSelected: '请先添加节点，再管理其内核与配置。',
  state: {
    offline: '离线',
    online: '在线',
    pending: '未接入',
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
    run_one: '{{action}} {{count}} 个节点',
    run_other: '{{action}} {{count}} 个节点',
    title_one: '{{action}} {{count}} 个节点？',
    title_other: '{{action}} {{count}} 个节点？',
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
    failed: '失败：{{reason}}',
    interrupts: {
      install_core: '安装核心不会改变正在运行的内容；需在各节点上选用它。',
      restart: '重启核心会短暂中断其连接。',
      stop: '停止核心会中断其连接，直到再次启动。',
      update: '每个节点更新后会连同核心重启，逐个进行；首个失败会停止其余节点。',
    },
    more: '等 {{count}} 个',
    noMatch: '没有匹配 {{target}} 的节点',
    ok: '完成',
    running_one: '正在对 {{count}} 个节点{{action}}…',
    running_other: '正在对 {{count}} 个节点{{action}}…',
    skipped: '因先前失败而跳过',
    summary: '成功 {{succeeded}} · 失败 {{failed}} · 跳过 {{skipped}}',
    toast: '{{action}}已完成',
  },
  confirm: '输入 y 确认',
  dismiss: '关闭输出',
  error: {
    ambiguous: '有多个节点名为 {{name}}，请使用节点 ID',
    noNode: '没有名为 {{name}} 的节点',
    noWindow: '没有编号为 {{position}} 的窗口',
    notFound: '未找到命令：{{command}}',
    unavailable: '{{command}} 在此处不可用',
  },
  help: {
    clear: '清空输出',
    exit: '退出登录',
    filter: '筛选节点：文本、state:online|offline|pending 或 tag:<标签>；留空清除',
    goto: '打开节点，可指定窗口',
    help: '列出命令',
    logout: '退出登录',
    number: '打开对应编号的窗口',
    open: '打开 nodes、subscriptions、logs 或 settings',
    restart: '重启节点的核心：节点名、通配如 hk-*、tag:<标签> 或 all',
    start: '启动节点的核心：节点名、通配如 hk-*、tag:<标签> 或 all',
    stop: '停止节点的核心：节点名、通配如 hk-*、tag:<标签> 或 all',
    update: '将节点逐个更新到面板的版本',
  },
  input: '命令',
  label: '命令行',
  output: '输出',
  shortcut: '命令行（{{keys}} 或 :）',
  usage: {
    filter: '用法：filter [文本 | state:online|offline|pending | tag:<标签>]',
    goto: '用法：goto <节点> [overview|cores|configuration|logs|settings]',
    open: '用法：open nodes|subscriptions|logs|settings',
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
