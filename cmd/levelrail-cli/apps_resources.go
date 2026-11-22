package main

// throughputFlags holds the disk and network limit flags of "apps scale".
type throughputFlags struct {
	ioRead, ioWrite string
	netMbit         int64
}

func (t throughputFlags) any() bool {
	return t.ioRead != "" || t.ioWrite != "" || t.netMbit != 0
}

// apply writes the set limits onto res; a value of 0 or "0" clears that limit.
func (t throughputFlags) apply(res *serviceResources) error {
	if t.netMbit < 0 {
		return newValidationError("--net-mbit must not be negative")
	}
	if t.ioRead != "" {
		n, err := parseOptionalMemory(t.ioRead)
		if err != nil {
			return newValidationError("--io-read: %v", err)
		}
		res.IOReadBps = n
	}
	if t.ioWrite != "" {
		n, err := parseOptionalMemory(t.ioWrite)
		if err != nil {
			return newValidationError("--io-write: %v", err)
		}
		res.IOWriteBps = n
	}
	if t.netMbit != 0 {
		res.NetMbit = t.netMbit
	}
	return nil
}
