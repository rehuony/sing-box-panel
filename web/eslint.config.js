import antfu from '@antfu/eslint-config';

export default antfu(
  {
    formatters: {
      css: true,
      html: true,
    },
    ignores: ['**/*.md', '**/*.json', 'pnpm-lock.yaml'],
    jsonc: false,
    markdown: false,
    react: true,
    stylistic: {
      braceStyle: '1tbs',
      indent: 2,
      quotes: 'single',
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
);
