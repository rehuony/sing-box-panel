export const login = {
  checking: '正在检查面板会话…',
  error: {
    empty: '请输入管理令牌以继续。',
    unauthorized: '该管理令牌未被接受。',
    unreachable: '无法连接面板，请重试。',
  },
  submit: { label: '打开面板', pending: '正在打开…' },
  subtitle: '本地管理控制台',
  token: {
    hint: '仅保存在此设备上。',
    label: '管理令牌',
    placeholder: '输入令牌',
  },
  unavailable: {
    description: '当前会话没有变化，请检查服务后重试。',
    retry: '重试',
    title: '无法连接面板服务。',
  },
} as const;
