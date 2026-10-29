package costestimate

// MonthlySaving is the estimated monthly drop when an app moves from its
// declared size to a suggested one, under the first provider of table.
type MonthlySaving struct {
	ProviderKey string
	USD         float64
}

// Saving prices both sizes with the same rate table and returns the
// difference. ok is false when no size is declared, the table is empty or
// the suggestion would not cost less, so callers never show a negative saving.
func Saving(currentMemoryBytes, currentNanoCPUs, suggestedMemoryBytes, suggestedNanoCPUs int64, table RateTable) (MonthlySaving, bool) {
	if len(table.Providers) == 0 || (currentMemoryBytes <= 0 && currentNanoCPUs <= 0) {
		return MonthlySaving{}, false
	}
	before := Estimate(Input{DeclaredMemoryBytes: currentMemoryBytes, DeclaredNanoCPUs: currentNanoCPUs}, table)
	after := Estimate(Input{DeclaredMemoryBytes: suggestedMemoryBytes, DeclaredNanoCPUs: suggestedNanoCPUs}, table)
	diff := roundCents(before.Providers[0].TotalUSD - after.Providers[0].TotalUSD)
	if diff <= 0 {
		return MonthlySaving{}, false
	}
	return MonthlySaving{ProviderKey: before.Providers[0].Key, USD: diff}, true
}
