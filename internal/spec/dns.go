package spec

// DNS holds the internal DNS options of a service.
type DNS struct {
	// Names are extra lowercase names under the mesh zone, resolved like the service's own name.
	Names []string `yaml:"names,omitempty" json:"names,omitempty"`
}
