/** Dot colour per certificate status, shared by the Pages and app wiring panels. */
export const TLS_DOT: Record<string, string> = {
  healthy: 'bg-success',
  expiring_soon: 'bg-warning',
  expired: 'bg-destructive',
}
