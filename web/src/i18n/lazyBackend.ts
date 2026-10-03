import type { BackendModule, ReadCallback } from 'i18next'

// Vite statically analyzes this template literal (the directory and
// extension are fixed, only lng/ns vary) and splits every matching
// locales/*/*.json into its own chunk, so a namespace's bundle only
// downloads the first time a component calls useTranslation(ns) for it,
// not on initial page load.
function importNamespace(language: string, namespace: string) {
  return import(`../locales/${language}/${namespace}.json`)
}

// Minimal i18next backend: dynamic import is the load, there is nothing
// to initialize or cache beyond what the browser's module cache already
// does for a repeated import() of the same chunk.
export const lazyBackend: BackendModule = {
  type: 'backend',
  init: () => {},
  read: (language: string, namespace: string, callback: ReadCallback) => {
    importNamespace(language, namespace)
      .then((mod: { default: object }) => {
        callback(null, mod.default)
      })
      .catch((err: unknown) => {
        callback(err instanceof Error ? err : String(err), null)
      })
  },
}
