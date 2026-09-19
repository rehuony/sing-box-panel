export const shell = {
  error: {
    badge: '上下文不可用',
    contextUnavailable: '面板上下文不可用，服务可能仍在启动。',
    description: '面板服务没有响应。',
    logout: '退出失败，当前会话仍然有效',
    retry: '重试',
  },
  loading: {
    description: '正在载入准确的运行身份与配置证据。',
    page: '正在载入工作台…',
    title: '正在读取面板上下文',
  },
  notSelected: '尚未选择',
  panelStatus: {
    loading: '正在检查面板状态',
    online: '面板在线',
    unavailable: '面板状态不可用',
  },
  skip: '跳到主要内容',
  version: 'v{{version}}',
} as const;

export const app = {
  title: 'Sing-Box Panel',
} as const;

export const nav = {
  panel: '面板配置',
  configuration: '配置管理',
  cores: '版本管理',
  dashboard: '仪表盘',
  observability: '运行日志',
  subscriptions: '订阅管理',
  tasks: '任务记录',
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
