// Doc-relative source path <-> in-app /help route path convention.
// Mirrored in scripts/build-docs-manifest.mjs (Node, runs outside the
// TS build), which must stay in sync with this file.
export function filePathToRoutePath(relFile: string): string {
  if (relFile === 'README.md') return '/'
  return `/${relFile.replace(/\.md$/, '')}`
}

export function routePathToFilePath(routePath: string): string {
  if (routePath === '/' || routePath === '') return 'README.md'
  return `${routePath.replace(/^\//, '')}.md`
}

/** Splits "path#anchor" into ["path", "#anchor"] (empty string when there's no anchor). */
export function splitHash(path: string): [base: string, hash: string] {
  const hashIndex = path.indexOf('#')
  if (hashIndex === -1) return [path, '']
  return [path.slice(0, hashIndex), path.slice(hashIndex)]
}
