package backup

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// RemoteObject is one object found under a bucket prefix.
type RemoteObject struct {
	Key  string
	Size int64
}

// Lister lists the objects under a key prefix in a backup target.
type Lister interface {
	List(ctx context.Context, dest Destination, prefix string) ([]RemoteObject, error)
}

// S3Lister is the real Lister, over ListObjectsV2.
type S3Lister struct{}

// List implements Lister, following pagination to the end.
func (S3Lister) List(ctx context.Context, dest Destination, prefix string) ([]RemoteObject, error) {
	client, err := newS3Client(ctx, dest)
	if err != nil {
		return nil, fmt.Errorf("backup: list %q in %q: %w", prefix, dest.Bucket, err)
	}
	var out []RemoteObject
	pager := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(dest.Bucket),
		Prefix: aws.String(prefix),
	})
	for pager.HasMorePages() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("backup: list %q in %q: %w", prefix, dest.Bucket, err)
		}
		for _, o := range page.Contents {
			out = append(out, RemoteObject{Key: aws.ToString(o.Key), Size: aws.ToInt64(o.Size)})
		}
	}
	return out, nil
}
