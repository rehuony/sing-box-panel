export const common = {
  back: '返回',
  actions: '操作',
  add: '添加',
  cancel: '取消',
  clear: '清空',
  close: '关闭',
  copy: '复制',
  create: '创建',
  delete: '删除',
  disabled: '已禁用',
  edit: '编辑',
  enabled: '已启用',
  loading: '正在加载',
  moveDown: '下移',
  moveUp: '上移',
  providerRequired: '{{hook}} 必须在 {{provider}} 中使用。',
  remove: '移除',
  requestFailed: '面板返回响应前，请求已失败。',
  retry: '重试',
  unsaved: {
    title: '放弃未保存的更改？',
    description: '离开后，当前的更改将不会保存。',
    keepEditing: '继续编辑',
    discard: '放弃更改',
  },
  viewLoadFailed: '无法加载此视图',
} as const;

export const bootstrap = {
  rootMissing: '未找到应用根节点',
} as const;
