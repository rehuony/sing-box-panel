export const login = {
  title: '登录你的账户',
  checking: '正在检查面板会话…',
  email: { label: '邮箱', placeholder: '请输入邮箱地址' },
  password: { label: '密码', placeholder: '输入你的密码', show: '显示密码', hide: '隐藏密码' },
  error: {
    email: '请输入有效的邮箱地址。',
    empty: '请输入密码以继续。',
    unauthorized: '邮箱或密码不正确。',
    rateLimited: '尝试次数过多，请稍候重试。',
    unreachable: '无法连接面板，请重试。',
  },
  submit: { label: '登录', pending: '正在登录…' },
  unavailable: {
    description: '当前会话没有变化，请检查服务后重试。',
    retry: '重试', title: '无法连接面板服务。',
  },
} as const;
