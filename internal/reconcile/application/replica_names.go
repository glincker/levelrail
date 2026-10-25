package application

// ReplicaContainerName returns the container name of replica index for the
// given image and restart nonce; index 0 equals ContainerName.
func ReplicaContainerName(serviceName, image, restartNonce string, index int) string {
	return replicaContainerName(serviceName, image, restartNonce, index)
}

// OwnsContainer reports whether name is a container this service could have
// created for any image or replica index.
func OwnsContainer(serviceName, name string) bool {
	return ownsContainer(serviceName, name)
}

// ReleaseKey returns the image-hash segment of one of serviceName's
// container names, shared by every replica of a release, or "" when name
// is not owned by serviceName.
func ReleaseKey(serviceName, name string) string {
	if !ownsContainer(serviceName, name) {
		return ""
	}
	return name[len(serviceName)+1 : len(serviceName)+1+hashLen]
}
