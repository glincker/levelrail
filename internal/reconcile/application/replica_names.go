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
