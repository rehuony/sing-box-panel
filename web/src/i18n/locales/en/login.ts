export const login = {
  title: 'Welcome back',
  checking: 'Checking panel session…',
  error: {
    empty: 'Enter the management token to continue.',
    unauthorized: 'That management token was not accepted.',
    unreachable: 'The panel could not be reached. Try again.',
  },
  submit: { label: 'Open panel', pending: 'Opening…' },
  subtitle: 'Verify your identity to enter your control center.',
  token: {
    hint: 'Stored only on this device.',
    label: 'Management token',
    placeholder: 'Enter management token',
    show: 'Show management token',
    hide: 'Hide management token',
  },
  unavailable: {
    description: 'Your session has not changed. Check the server and try again.',
    retry: 'Try again',
    title: 'The panel service could not be reached.',
  },
} as const;
