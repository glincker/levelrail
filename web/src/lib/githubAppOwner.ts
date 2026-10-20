export type GitHubAppOwnerKind = 'personal' | 'organization'

// GitHub org logins: alphanumerics and hyphens, 1 to 39 chars, no leading hyphen.
const ORG_LOGIN_PATTERN = /^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$/

export function isValidOrgLogin(login: string): boolean {
  return ORG_LOGIN_PATTERN.test(login.trim())
}

// registerStartParams builds the query for GET /api/v1/github-app/register/start.
export function registerStartParams(input: {
  name: string
  instanceURL: string
  ownerKind: GitHubAppOwnerKind
  orgLogin: string
  allowOthers: boolean
}): URLSearchParams {
  const params = new URLSearchParams()
  if (input.name.trim() !== '') {
    params.set('name', input.name.trim())
  }
  if (input.instanceURL.trim() !== '') {
    params.set('instance_url', input.instanceURL.trim())
  }
  if (input.ownerKind === 'organization' && input.orgLogin.trim() !== '') {
    params.set('owner', input.orgLogin.trim())
  }
  if (input.allowOthers) {
    params.set('public', 'true')
  }
  return params
}
