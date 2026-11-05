export function deployReceiptUrl(
  appName: string,
  deployId: string,
  format: 'json' | 'md' = 'json',
): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/deployments/${encodeURIComponent(deployId)}/receipt?format=${format}`
}
