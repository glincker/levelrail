import type { BackendModule, ReadCallback } from 'i18next'

type Loader = () => Promise<{
  load: (namespace: string) => Promise<{ default: object }>
}>

// A new language adds one module in ./loaders and one line here (a test fails if either is missing).
const loaders: Record<string, Loader> = {
  en: () => import('./loaders/en'),
  es: () => import('./loaders/es'),
  de: () => import('./loaders/de'),
  fr: () => import('./loaders/fr'),
  'pt-BR': () => import('./loaders/pt-BR'),
  ja: () => import('./loaders/ja'),
  'zh-CN': () => import('./loaders/zh-CN'),
}

export const LOADER_LANGUAGES = Object.keys(loaders)

function importNamespace(language: string, namespace: string) {
  const loader = loaders[language]
  if (!loader) return Promise.reject(new Error(`no loader for ${language}`))
  return loader().then((m) => m.load(namespace))
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
        // A locale may ship only some namespaces: missing keys fall back to English.
        if (language !== 'en') {
          callback(null, {})
          return
        }
        callback(err instanceof Error ? err : String(err), null)
      })
  },
}
