export const common = {
  actions: 'Actions',
  add: 'Add',
  cancel: 'Cancel',
  clear: 'Clear',
  clearSearch: 'Clear search',
  close: 'Close',
  copy: 'Copy',
  create: 'Create',
  delete: 'Delete',
  edit: 'Edit',
  loading: 'Loading',
  moveDown: 'Move down',
  moveUp: 'Move up',
  providerRequired: '{{hook}} must be used within {{provider}}.',
  remove: 'Remove',
  requestFailed: 'The request failed before the panel returned a response.',
  unsaved: {
    title: 'Discard unsaved changes?',
    description: 'Your changes will be lost if you leave this page.',
    discard: 'Discard changes',
  },
} as const;

export const bootstrap = {
  rootMissing: 'Root element was not found',
} as const;
