// Maps doc file paths and route paths to docs site URLs and vice versa.
// Handles: index.md -> '', nested paths, .md extension stripping.

export function filePathToRoutePath(filePath: string): string {
  let path = filePath.replace(/\.md$/, '')
  if (path === 'index') {
    path = '/'
  } else if (!path.startsWith('/')) {
    path = `/${path}`
  }
  return path
}

export function routePathToDocsSiteUrl(
  routePath: string,
  docsBaseUrl: string,
): string {
  if (!docsBaseUrl) return ''
  const cleanUrl = docsBaseUrl.replace(/\/$/, '')
  const cleanPath = routePath === '/' ? '' : routePath
  return `${cleanUrl}${cleanPath}`
}

export function routePathWithHash(routePath: string, hash?: string): string {
  return hash ? `${routePath}#${hash}` : routePath
}
