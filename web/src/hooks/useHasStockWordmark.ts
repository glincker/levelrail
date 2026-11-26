import { useBrand } from './useBrand'

// The bundled wordmark is the product's own artwork, so it only shows when
// the brand name is still the stock one; a rebrand falls back to its own mark.
const STOCK_BRAND_NAME = 'Levelrail'

export function useHasStockWordmark(): boolean {
  return useBrand().Name === STOCK_BRAND_NAME
}
