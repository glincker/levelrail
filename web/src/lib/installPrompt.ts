export const DEMO_INSTALL_EVENT = 'levelrail:demo-install'

/**
 * Asks the demo shell to open its Install dialog. A listener claims the request
 * with preventDefault; without one, the install docs open so the click is never dead.
 */
export function openDemoInstall(docsUrl?: string): void {
  const event = new CustomEvent(DEMO_INSTALL_EVENT, { cancelable: true })
  const claimed = !window.dispatchEvent(event)
  if (!claimed && docsUrl) window.open(docsUrl, '_blank', 'noopener,noreferrer')
}
