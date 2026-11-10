package spec

import "fmt"

// imageKeepMin keeps the previous successful release as a rollback target.
const imageKeepMin = 2

// imageKeepMax matches the ceiling of the global images-kept setting.
const imageKeepMax = 1000

// ImageRetention is a service's image keep-last count for the image cleanup.
type ImageRetention struct {
	// Keep is how many newest successful deploy images stay; 0 follows the global setting.
	Keep int `yaml:"keep"`
}

// Validate checks the block; a nil block is valid.
func (r *ImageRetention) Validate(service string) error {
	if r == nil || r.Keep == 0 {
		return nil
	}
	if r.Keep < imageKeepMin || r.Keep > imageKeepMax {
		return fmt.Errorf("spec: service %q: images.keep must be 0 (follow the global setting) or from %d to %d", service, imageKeepMin, imageKeepMax)
	}
	return nil
}
