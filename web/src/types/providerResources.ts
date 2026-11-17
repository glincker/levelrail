export interface ProviderResource {
  id: string
  name: string
  region: string
  size: string
  status: string
  public_ipv4: string
  created_at: string
  tags: string[] | null
  estimated_monthly_price: number | null
  currency: string
  managed_by_levelrail: boolean
  enrolled_node: { id: string; name: string } | null
}

export interface ProviderResources {
  provider: string
  account_id: string
  generated_at: string
  cached: boolean
  count: number
  resources: ProviderResource[]
}

export type SpendState = 'ok' | 'scope_missing' | 'unavailable'

export interface SpendValue {
  value: string
  source: string
  data_as_of: string | null
}

interface SpendSectionBase {
  state: SpendState
  needs?: string
}

export interface ProviderSpend {
  provider: string
  account_id: string
  currency: string
  generated_at: string
  cached: boolean
  balance: SpendSectionBase & {
    account_balance: SpendValue | null
    month_to_date_balance: SpendValue | null
    month_to_date_usage: SpendValue | null
    generated_at: string | null
  }
  last_invoice: SpendSectionBase & {
    amount: SpendValue | null
    invoice_id?: string
    description?: string
    date: string | null
  }
  estimate: SpendSectionBase & {
    monthly_total: SpendValue | null
    resource_count: number
    priced_count: number
  }
}
