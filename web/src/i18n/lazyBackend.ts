import type { BackendModule, ReadCallback } from 'i18next'

// Fixed dir/extension lets Vite split each locales/*/*.json into its own chunk.
function importNamespace(language: string, namespace: string) {
  return import(`../locales/${language}/${namespace}.json`)
}

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
