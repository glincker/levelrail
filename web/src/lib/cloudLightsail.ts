// Only managed servers that are not mid transition can be changed from the dashboard.
export function lightsailCanChange(i: {
  managed: boolean
  state: string
}): boolean {
  return i.managed && (i.state === 'running' || i.state === 'stopped')
}
