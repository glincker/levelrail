// The exact command that mints a browser session; REGION stays literal until one is entered.
export function ociSessionCommand(region: string): string {
  const r = region.trim() === '' ? 'REGION' : region.trim()
  return `oci session authenticate --region ${r} --profile-name DEFAULT`
}
