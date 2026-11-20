package catalog

// Count is the single computed template count: every doc, API and site figure
// reads this so they cannot drift apart.
func Count() int { return len(Templates) }
