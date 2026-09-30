export const login = {
  title: 'Sign in to your account',
  checking: 'Checking panel session…',
  email: { label: 'Email', placeholder: 'Enter your email address' },
  password: { label: 'Password', placeholder: 'Enter your password', show: 'Show password', hide: 'Hide password' },
  error: {
    email: 'Enter a valid email address.',
    empty: 'Enter your password to continue.',
    unauthorized: 'The email or password is incorrect.',
    rateLimited: 'Too many attempts. Wait a moment and try again.',
    unreachable: 'The panel could not be reached. Try again.',
  },
  submit: { label: 'Sign in', pending: 'Signing in…' },
  unavailable: {
    description: 'Your session has not changed. Check the server and try again.',
    retry: 'Try again', title: 'The panel service could not be reached.',
  },
} as const;
