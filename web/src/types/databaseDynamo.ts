// Shapes of the DynamoDB viewer routes (internal/api/database_viewer_dynamo.go).

export type DynamoAttr = Record<string, unknown>
export type DynamoItem = Record<string, DynamoAttr>

export interface DynamoKeyPart {
  AttributeName: string
  KeyType: 'HASH' | 'RANGE'
}

export interface DynamoIndexInfo {
  IndexName: string
  KeySchema: DynamoKeyPart[]
}

export interface DynamoTable {
  TableName: string
  TableStatus?: string
  ItemCount?: number
  TableSizeBytes?: number
  KeySchema: DynamoKeyPart[]
  AttributeDefinitions?: { AttributeName: string; AttributeType: string }[]
  GlobalSecondaryIndexes?: DynamoIndexInfo[]
  LocalSecondaryIndexes?: DynamoIndexInfo[]
  TimeToLive?: { TimeToLiveStatus?: string; AttributeName?: string }
}

export interface DynamoTablesResponse {
  tables: DynamoTable[]
  names: string[]
  truncated: boolean
  note: string
}

export type DynamoSortOp = '=' | '<' | '<=' | '>' | '>=' | 'begins_with'

export interface DynamoReadParams {
  op: 'query' | 'scan'
  table: string
  index?: string
  key_name?: string
  key_value?: DynamoAttr
  sort?: { name: string; op: DynamoSortOp; value: DynamoAttr }
  limit?: number
  exclusive_start_key?: DynamoItem
  confirm?: boolean
}

export interface DynamoItemsResponse {
  items: DynamoItem[]
  count: number
  scanned_count: number
  last_evaluated_key?: DynamoItem
  truncated: boolean
  expensive: boolean
  duration_ms: number
}
