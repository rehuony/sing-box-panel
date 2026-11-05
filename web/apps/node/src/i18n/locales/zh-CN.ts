import type { NodeTranslation } from './en';

export const zhCN: NodeTranslation = {
  bootstrap: {
    rootMissing: '缺少应用根元素。',
  },
  common: {
    cancel: '取消',
    close: '关闭',
    providerRequired: '{{hook}} 必须在 {{provider}} 内使用。',
    retry: '重试',
    unavailable: '—',
  },
  console: {
    checking: '正在检查控制台会话…',
    disabled: {
      description: '此节点的控制台已关闭。使用 sing-box-node console enable 开启。',
      title: '控制台已关闭',
    },
    title: '节点控制台',
    unavailable: {
      description: '节点没有响应。请确认 sing-box-node 正在运行。',
      title: '控制台不可用',
    },
  },
  login: {
    error: {
      empty: '请输入控制台密码。',
      rateLimited: '尝试次数过多，请等待一分钟后重试。',
      unauthorized: '密码不正确。',
      unreachable: '节点没有响应，请重试。',
    },
    password: '控制台密码',
    submit: '登录',
    title: '登录节点控制台',
  },
  status: {
    connection: {
      lastError: '最近错误',
      retryAt: '下次尝试',
      since: '开始于',
      stateLabel: '状态',
      state: {
        connected: '已连接',
        connecting: '连接中',
        disconnected: '已断开',
        unenrolled: '未注册',
      },
      title: '连接',
    },
    node: {
      platform: '平台',
      protocol: '协议',
      title: '节点',
      version: '版本',
    },
    panel: {
      endpoint: '面板地址',
      nodeId: '节点 ID',
      title: '面板',
      trust: {
        label: '信任方式',
        pin: '固定证书',
        system: '系统证书颁发机构',
      },
    },
    signOut: '退出登录',
    stale: '节点没有响应，显示的是上一次的状态。',
    unenrolled: {
      description: '先在面板中添加此节点，再在本机运行面板给出的命令：sing-box-node enroll --code <code>。',
      title: '未注册',
    },
  },
};
