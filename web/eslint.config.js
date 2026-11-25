import antfu from '@antfu/eslint-config';

export default antfu(
  {
    formatters: {
      css: true,
      html: true,
    },
    ignores: [
      '**/*.md',
      '**/*.json',
      'pnpm-lock.yaml',
      '**/dist/**',
      '**/src/api/generated/**',
    ],
    jsonc: false,
    markdown: false,
    react: true,
    stylistic: {
      braceStyle: '1tbs',
      quotes: 'single',
      indent: 2,
      semi: true,
    },
    toml: false,
    typescript: true,
    yaml: false,
  },
  {
    files: ['**/*.{js,jsx,ts,tsx}'],
    rules: {
      'antfu/consistent-list-newline': 'off',
      'antfu/if-newline': 'off',
      'import/consistent-type-specifier-style': ['error', 'top-level'],
      'no-console': 'off',
      'perfectionist/sort-enums': [
        'error',
        {
          fallbackSort: { order: 'asc', type: 'alphabetical' },
          order: 'asc',
          type: 'line-length',
        },
      ],
      'perfectionist/sort-exports': [
        'error',
        {
          fallbackSort: { order: 'desc', type: 'alphabetical' },
          order: 'desc',
          type: 'line-length',
        },
      ],
      'perfectionist/sort-imports': [
        'error',
        {
          fallbackSort: { order: 'asc', type: 'alphabetical' },
          order: 'asc',
          type: 'line-length',
        },
      ],
      'perfectionist/sort-interfaces': [
        'error',
        {
          fallbackSort: { order: 'asc', type: 'alphabetical' },
          order: 'asc',
          type: 'line-length',
        },
      ],
      'style/arrow-parens': 'off',
      'style/brace-style': ['error', '1tbs'],
      'style/jsx-closing-bracket-location': 'off',
      'style/jsx-curly-newline': 'off',
      'style/jsx-first-prop-new-line': 'off',
      'style/jsx-max-props-per-line': 'off',
      'style/jsx-quotes': ['error', 'prefer-single'],
      'style/max-len': [
        'error',
        {
          code: 120,
          ignoreStrings: true,
          ignoreTemplateLiterals: true,
          ignoreUrls: true,
        },
      ],
      'style/nonblock-statement-body-position': ['error', 'beside'],
    },
  },
  // Shared code serves both applications, and each application ships alone:
  // the node binary embeds only the node console.
  {
    files: ['packages/shared/**/*.{js,jsx,ts,tsx}'],
    rules: {
      'no-restricted-imports': ['error', {
        patterns: [{ group: ['@/*', '**/apps/**'], message: 'Shared code must not import an application.' }],
      }],
    },
  },
  {
    files: ['apps/panel/**/*.{js,jsx,ts,tsx}'],
    rules: {
      'no-restricted-imports': ['error', {
        patterns: [{ group: ['**/apps/node/**', '**/node/src/**'], message: 'The panel must not import the node console.' }],
      }],
    },
  },
  {
    files: ['apps/node/**/*.{js,jsx,ts,tsx}'],
    rules: {
      'no-restricted-imports': ['error', {
        patterns: [{ group: ['**/apps/panel/**', '**/panel/src/**'], message: 'The node console must not import the panel.' }],
      }],
    },
  },
);
