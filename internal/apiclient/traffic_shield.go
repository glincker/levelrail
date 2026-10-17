package apiclient

// TrafficShieldReason is how many requests the abuse shield turned away for one reason.
type TrafficShieldReason struct {
	Reason string  `json:"reason"`
	Count  float64 `json:"count"`
}

// TrafficShield is the abuse shield's blocked-request counts over the window.
type TrafficShield struct {
	Blocked  float64               `json:"blocked"`
	ByReason []TrafficShieldReason `json:"by_reason"`
}
