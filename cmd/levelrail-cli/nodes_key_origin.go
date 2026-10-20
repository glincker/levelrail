package main

// keyOriginLabel words the API's key_origin: "reenroll_required" means the
// control plane once generated this key, so the node must be re-enrolled.
func keyOriginLabel(origin string) string {
	if origin == "reenroll_required" {
		return "the control plane (re-enroll required)"
	}
	return origin
}
