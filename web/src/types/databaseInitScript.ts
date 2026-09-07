// Wire types for GET/POST/PUT/DELETE
// /api/v1/databases/{name}/init-scripts(/{id}) (internal/api/database_init_scripts.go):
// named SQL/shell files mounted into a managed database's container at
// /docker-entrypoint-initdb.d, run once on first container start
// against an empty data volume (postgres/mysql/mariadb/mongodb only).

export interface DatabaseInitScript {
  id: string
  database_name: string
  filename: string
  content: string
  created_at: string
  updated_at: string
}

export interface SetDatabaseInitScriptRequest {
  filename: string
  content: string
}
