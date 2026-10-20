// Wire shape of GET/POST /api/v1/apps/{name}/git-remote.
export interface GitRemoteResource {
  configured: boolean
  url: string
  remote_name: string
  username: string
  branch: string
  created_at?: string
  rotated_at?: string
  last_push_at?: string
  last_push_commit?: string
  last_push_result?: string
  max_push_bytes: number
  // Set only on the call that issued the token.
  token?: string
  add_command?: string
}
