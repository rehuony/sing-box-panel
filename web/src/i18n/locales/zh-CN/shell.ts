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
