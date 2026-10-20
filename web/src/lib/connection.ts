const LOOPBACK_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]', '::1'])

// True when the page was loaded over plain HTTP from a non-loopback host,
// meaning credentials and the session cookie cross the network unencrypted.
export function isInsecureRemoteConnection(loc: {
  protocol: string
  hostname: string
}): boolean {
  return loc.protocol === 'http:' && !LOOPBACK_HOSTS.has(loc.hostname)
}
