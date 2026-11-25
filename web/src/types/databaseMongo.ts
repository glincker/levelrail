// Shapes of the MongoDB viewer routes (internal/api/database_viewer_mongo.go).

export type JsonValue =
  string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue }

export interface MongoDbInfo {
  kind: 'db'
  name: string
  sizeOnDisk: number
}

export interface MongoIndexInfo {
  name: string
  key: Record<string, JsonValue>
  unique: boolean
}

export interface MongoCollectionInfo {
  kind: 'collection'
  db: string
  name: string
  type: string
  count: number
  size: number
  storageSize: number
  indexes: MongoIndexInfo[]
  error?: string
}

export interface MongoCollectionsResponse {
  databases: MongoDbInfo[]
  collections: MongoCollectionInfo[]
  truncated: boolean
  fingerprint: string
}

export interface MongoFindParams {
  database: string
  collection: string
  filter?: JsonValue
  sort?: JsonValue
  projection?: JsonValue
  limit: number
  after?: JsonValue
}

export interface MongoFindResponse {
  documents: JsonValue[]
  truncated: boolean
  next_after?: JsonValue
  fingerprint: string
  duration_ms: number
}
