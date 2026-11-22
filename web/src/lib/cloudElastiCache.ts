// Only managed caches that are running can be changed from the dashboard.
export function cacheCanChange(i: {
  managed: boolean
  status: string
}): boolean {
  return i.managed && i.status === 'available'
}
