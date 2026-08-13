import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import prettierConfig from 'eslint-config-prettier'

export default tseslint.config(
  {
    ignores: ['dist', 'src/routeTree.gen.ts'],
  },
  {
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommendedTypeChecked,
      reactHooks.configs.flat['recommended-latest'],
      reactRefresh.configs.vite,
    ],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2023,
      globals: globals.browser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      // CLAUDE.md 7: "TypeScript strict mode, no any." Flags any explicit
      // `any` usage, including implicit escapes like `as any`.
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-unsafe-assignment': 'error',
      '@typescript-eslint/no-unsafe-member-access': 'error',
      '@typescript-eslint/no-unsafe-call': 'error',
      '@typescript-eslint/no-floating-promises': 'error',
      // Not opted into React Compiler in this pass (no
      // babel-plugin-react-compiler configured), so the react-hooks v7
      // rules that only matter under the compiler are noise here.
      'react-hooks/incompatible-library': 'off',
    },
  },
  // TanStack Router's file-based routing convention has every route file
  // export a non-component `Route` object alongside its `component`.
  // react-refresh/only-export-components does not know that pattern and
  // flags it as a fast-refresh hazard; the router's own Vite plugin
  // handles route file HMR, so the rule does not apply here.
  {
    files: ['src/routes/**/*.{ts,tsx}'],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
  // shadcn/ui's generated components (src/components/ui/**, e.g.
  // button.tsx, tabs.tsx) follow the upstream registry's convention of
  // co-exporting a cva() variants function alongside the component in the
  // same file. react-refresh/only-export-components flags that as a fast
  // refresh hazard; these files are vendored from the shadcn registry
  // (regenerated via `npx shadcn add --overwrite`, not hand-edited), so
  // fighting the upstream shape here would just be undone on the next
  // `shadcn add`.
  {
    files: ['src/components/ui/**/*.{ts,tsx}'],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
  // Plain config files run under Node, not the browser/DOM lib, and are
  // not part of the type-checked app project.
  {
    files: ['*.config.{js,ts}'],
    extends: [tseslint.configs.disableTypeChecked],
    languageOptions: {
      globals: globals.node,
    },
  },
  // eslint-config-prettier last: turns off any ESLint stylistic rule that
  // would otherwise fight Prettier's formatting.
  prettierConfig,
)
