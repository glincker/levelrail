// Wire types for POST /api/v1/databases/{name}/clone
// (internal/api/database_clone_now.go's cloneNowResource/cloneNowRequest):
// the one-click counterpart to types/cloneRestore.ts's "restore as new
// database" action that takes a fresh backup as part of the same call
// instead of requiring one to already exist.

export interface CloneNowResult {
  source_database_name: string
  new_database_name: string
  target_id: string
  backup_history_id: string
  clone_restore_id: string
}

export interface CloneNowRequest {
  new_name: string
  target_id?: string
}
