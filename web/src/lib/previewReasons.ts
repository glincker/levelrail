// Plain-language text for the reason codes internal/preview stores on a
// deploy preview that was skipped or failed.

const REASON_TEXT: Record<string, string> = {
  low_ram: 'Skipped: the server was low on free memory.',
  low_disk: 'Skipped: the server was low on free disk space.',
  browser_image_unavailable: 'Skipped: the browser image could not be pulled.',
  no_app_network: 'Skipped: the app has no private network to capture from.',
  remote_node: 'Skipped: the app runs on another node.',
  unreachable: 'Skipped: the app did not answer on its network.',
  http_status: 'Skipped: the page returned an error status.',
  auth_wall: 'Skipped: the page needs a login.',
  blank_image: 'Skipped: the page rendered blank.',
  bad_image: 'The screenshot could not be processed.',
  timeout: 'The capture took too long.',
  capture_failed: 'The capture failed.',
}

export function previewReasonText(reason?: string): string {
  return (reason && REASON_TEXT[reason]) || 'No preview was captured.'
}
