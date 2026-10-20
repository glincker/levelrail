export interface StaticBuildValue {
  command: string
  publish: string
}

// staticBuildMismatch is true when exactly one of the pair is filled, which
// the server rejects: a command needs somewhere to publish from.
export function staticBuildMismatch(v: StaticBuildValue): boolean {
  return (v.command.trim() === '') !== (v.publish.trim() === '')
}
