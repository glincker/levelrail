package dbupgrade

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

// ChannelStore loads alerting notification channels.
type ChannelStore interface {
	GetNotificationChannel(ctx context.Context, id string) (*alerting.NotificationChannel, error)
}

// NoticeSender delivers plain text to a channel, decrypting its credential
// at send time; *alerting.DeployDispatcher satisfies it.
type NoticeSender interface {
	SendNotice(ctx context.Context, c alerting.NotificationChannel, text string) error
}

// ChannelNotifier sends upgrade notices through the alert channels an
// operator already connected (Slack, Discord, email, webhooks and so on).
type ChannelNotifier struct {
	Channels ChannelStore
	Sender   NoticeSender
}

// Notify sends text to every enabled channel; one failure does not stop the rest.
func (n ChannelNotifier) Notify(ctx context.Context, channelIDs []string, text string) error {
	var errs []error
	for _, id := range channelIDs {
		ch, err := n.Channels.GetNotificationChannel(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Errorf("channel %q: %w", id, err))
			continue
		}
		if !ch.Enabled {
			continue
		}
		if err := n.Sender.SendNotice(ctx, *ch, text); err != nil {
			errs = append(errs, fmt.Errorf("channel %q: %w", id, err))
		}
	}
	return errors.Join(errs...)
}
