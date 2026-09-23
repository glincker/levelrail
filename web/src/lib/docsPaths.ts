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
