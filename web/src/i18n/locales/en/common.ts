export const common = {
  back: 'Back',
  actions: 'Actions',
  add: 'Add',
  cancel: 'Cancel',
  clear: 'Clear',
  close: 'Close',
  copy: 'Copy',
  create: 'Create',
  delete: 'Delete',
  disabled: 'Disabled',
  edit: 'Edit',
  enabled: 'Enabled',
  loading: 'Loading',
  moveDown: 'Move down',
  moveUp: 'Move up',
  providerRequired: '{{hook}} must be used within {{provider}}.',
  remove: 'Remove',
  requestFailed: 'The request failed before the panel returned a response.',
  retry: 'Retry',
  unsaved: {
    title: 'Discard unsaved changes?',
    description: 'Your changes will be lost if you leave this page.',
    keepEditing: 'Keep editing',
    discard: 'Discard changes',
  },
  viewLoadFailed: 'This view could not be loaded',
} as const;

export const bootstrap = {
  rootMissing: 'Root element was not found',
} as const;
