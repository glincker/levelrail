package spec

// WithNextCache applies a stored next_cache setting over the spec value; empty keeps the spec.
// Every deploy trigger shares it so the stored choice wins the same way everywhere.
func (svc Service) WithNextCache(cache string) Service {
	if cache == "" {
		return svc
	}
	next := Next{}
	if svc.Next != nil {
		next = *svc.Next
	}
	next.Cache = cache
	svc.Next = &next
	return svc
}
