const SETUP_HASH_KEY = 'setup'

/** setupTokenFromHash reads the installer's #setup=<token> fragment, which never reaches the server, logs or referrers. */
export function setupTokenFromHash(hash: string): string | undefined {
  const token = new URLSearchParams(hash.replace(/^#/, '')).get(SETUP_HASH_KEY)
  return token ? token : undefined
}
