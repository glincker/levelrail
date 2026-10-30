package docker

import "context"

// DigestVerifier checks that a digest-pinned reference is still known to its
// registry. exists is false only on a definitive "no such manifest".
type DigestVerifier interface {
	ImageDigestExists(ctx context.Context, ref string) (exists bool, err error)
}

// ImageDigestExists implements DigestVerifier with a manifest lookup only,
// no layers are pulled.
func (c *Client) ImageDigestExists(ctx context.Context, ref string) (bool, error) {
	_, err := c.cli.DistributionInspect(ctx, ref, "")
	if err == nil {
		return true, nil
	}
	if registrySaysMissing(err) {
		return false, nil
	}
	return false, err
}
