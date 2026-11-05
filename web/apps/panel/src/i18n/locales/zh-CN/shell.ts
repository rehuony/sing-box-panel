export const shell = {
  error: {
    logout: '退出失败，当前会话仍然有效',
    retry: '重试',
  },
  loading: {
    page: '正在载入工作台…',
  },
  openNavigation: '打开导航',
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

export const sidebar = {
  mobileDescription: '显示移动端导航。',
  title: '导航',
  toggle: '切换导航',
} as const;

export const theme = {
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
