package main

// publishedPortHint is the one-line next step for each published-port
// refusal code; the server message itself is printed unchanged above it.
func publishedPortHint(code string) string {
	switch code {
	case "published_port_remote_node":
		return "place the app on the control plane node, host ports publish only there"
	case "published_port_conflict":
		return "choose another host port, this one is already published"
	case "published_port_in_use":
		return "another process on this server already uses that port; choose another host port or stop that process (a 127.0.0.1 bind is not blocked by system resolvers)"
	case "published_port_invalid":
		return `use "HOST:CONTAINER/udp", for example "51820:51820/udp"`
	}
	return ""
}
