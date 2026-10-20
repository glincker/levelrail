// Lazy, per-file chunks for every bundled doc's raw markdown: import.meta.
// glob (non-eager) keeps each page out of the /help route's own chunk,
// fetched only when that page is actually opened, which is what keeps a
// 560kB docs tree from blowing the per-chunk bundle budget. Keys come back
// relative to this file ("../../../docs/getting-started.md"); normalized
// to the same doc-relative path used everywhere else ("getting-started.md").
const rawLoaders = import.meta.glob('../../../docs/**/*.md', {
  query: '?raw',
  import: 'default',
}) as Record<string, () => Promise<string>>

const loadersByFile = new Map<string, () => Promise<string>>()
for (const [key, loader] of Object.entries(rawLoaders)) {
  const relFile = key.replace('../../../docs/', '')
  loadersByFile.set(relFile, loader)
}

export function loadDocContent(relFile: string): Promise<string> {
  const loader = loadersByFile.get(relFile)
  if (!loader) {
    return Promise.reject(new Error(`no bundled doc for ${relFile}`))
  }
  return loader()
}
