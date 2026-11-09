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
  connection: {
    retryAt: '下次尝试',
    retrying: '正在进行',
    state: {
      connected: '已连接',
      connecting: '连接中',
      disconnected: '已断开',
      unenrolled: '未接入',
    },
  },
  console: {
    checking: '正在检查控制台会话…',
    disabled: {
      description: '此节点的控制台已关闭。使用 sing-box-node console enable 开启。',
      title: '控制台已关闭',
    },
    loading: '正在读取节点状态…',
    signOut: '退出登录',
    stale: '节点没有响应，显示的是上一次的状态。',
    staleShort: '未能刷新',
    title: '节点控制台',
    unavailable: {
      description: '节点没有响应。请确认 sing-box-node 正在运行。',
      title: '控制台不可用',
    },
  },
  danger: {
    more: '更多操作',
    reenroll: '重新接入',
    reenrollConfirm: '此节点已接入 {{address}}。用新代码接入会替换当前的接入并结束现有会话；运行时保留已应用的配置。',
    reenrollTitle: '重新接入此节点',
    unenroll: '解除接入',
    unenrollConfirm: '节点会结束与 {{address}} 的会话并删除接入信息。面板会保留该节点并显示为离线，直到在面板中移除；运行时保留已应用的配置。',
    unenrollFailed: '节点无法解除接入，请重试。',
    unenrollTitle: '解除与 {{address}} 的接入？',
  },
  details: {
    address: '地址',
    node: '此节点',
    copied: '已复制',
    copy: '复制{{label}}',
    nodeId: '节点 ID',
    panel: '面板',
    platform: '平台',
    protocol: '协议',
    trust: {
      label: '信任方式',
      pin: '固定证书',
      system: '系统证书颁发机构',
    },
    version: '版本',
  },
  enroll: {
    code: '接入代码',
    description: '先在面板中添加此节点，再粘贴面板显示的一次性代码。代码只能使用一次，一小时后过期。',
    error: {
      empty: '请粘贴面板给出的接入代码。',
      failed: '节点无法接入，请重试。',
      invalid: '这不是有效的接入代码，请从面板重新复制。',
      rejected: '面板没有接受此代码，它可能已被使用或已过期；请在面板中重新签发。',
      unreachable: '代码中的面板没有响应。请确认本机可以访问它，然后重试。',
    },
    placeholder: '粘贴面板给出的代码',
    submit: '接入',
    title: '接入此节点',
  },
  language: {
    english: 'English',
    label: '语言',
    menu: '打开语言菜单',
    simplifiedChinese: '简体中文',
  },
  login: {
    error: {
      empty: '请输入控制台密码。',
      rateLimited: '尝试次数过多，请等待一分钟后重试。',
      unauthorized: '密码不正确。',
      unreachable: '节点没有响应，请重试。',
    },
    hide: '隐藏密码',
    password: '控制台密码',
    show: '显示密码',
    submit: '登录',
    title: '登录节点控制台',
  },
  theme: {
    cycle: '当前主题：{{current}}。切换为{{next}}',
    preference: {
      dark: '深色',
      light: '亮色',
      system: '系统',
    },
  },
};
