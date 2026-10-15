// Static sibling routes that share a prefix with a `$name` route must never be
// read as a resource name, or the scoped sidebar loads a resource that does not exist.
const RESERVED_SEGMENTS = new Set(['new'])

function scopedName(prefix: string, pathname: string): string | undefined {
  const match = new RegExp(`^/${prefix}/([^/]+)`).exec(pathname)
  const name = match?.[1]
  if (name === undefined || RESERVED_SEGMENTS.has(name)) return undefined
  return name
}

/** appScopeName is the app a /apps/<name>/... path belongs to, if any. */
export function appScopeName(pathname: string): string | undefined {
  return scopedName('apps', pathname)
}

/** databaseScopeName is the database a /databases/<name>/... path belongs to, if any. */
export function databaseScopeName(pathname: string): string | undefined {
  return scopedName('databases', pathname)
}
