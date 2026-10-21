package email

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// sesSender sends via AWS SES v2's SendEmail API, the same AWS SDK v2
// dependency and static-credentials pattern internal/backup's S3Uploader
// already establishes for backup target credentials.
type sesSender struct {
	cfg SESConfig
}

func (s sesSender) Send(ctx context.Context, to, subject, body string) error {
	_, err := s.sendID(ctx, to, subject, body)
	return err
}

func (s sesSender) sendID(ctx context.Context, to, subject, body string) (string, error) {
	client, err := s.client(ctx)
	if err != nil {
		return "", fmt.Errorf("email: build ses client: %w", err)
	}

	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(s.cfg.From),
		Destination:      &types.Destination{ToAddresses: []string{to}},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: aws.String(subject)},
				Body:    &types.Body{Text: &types.Content{Data: aws.String(body)}},
			},
		},
	}
	if s.cfg.ReplyTo != "" {
		input.ReplyToAddresses = []string{s.cfg.ReplyTo}
	}
	out, err := client.SendEmail(ctx, input)
	if err != nil {
		return "", fmt.Errorf("email: send via ses: %w", err)
	}
	return aws.ToString(out.MessageId), nil
}

// client builds an SES v2 client scoped to s.cfg's own static
// credentials: a specific operator-entered SES identity, never this
// process's own ambient AWS identity, the same reasoning
// internal/backup's newS3Client already establishes.
func (s sesSender) client(ctx context.Context) (*sesv2.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(s.cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			s.cfg.AccessKeyID, s.cfg.SecretAccessKey, "",
		)),
	)
	if err != nil {
		return nil, err
	}
	return sesv2.NewFromConfig(cfg), nil
}
