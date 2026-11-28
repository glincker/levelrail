package apiclient

import (
	"context"
	"net/http"
	"time"
)

// AnnouncementLink is one outbound link on an announcement.
type AnnouncementLink struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// Announcement is one verified feed item with the caller's own marks.
type Announcement struct {
	ID           string             `json:"id"`
	Tier         string             `json:"tier"`
	Title        string             `json:"title"`
	Body         string             `json:"body"`
	AppliesTo    string             `json:"applies_to,omitempty"`
	CTA          string             `json:"cta,omitempty"`
	Links        []AnnouncementLink `json:"links"`
	IssuedAt     time.Time          `json:"issued_at"`
	ExpiresAt    time.Time          `json:"expires_at"`
	Read         bool               `json:"read"`
	Dismissed    bool               `json:"dismissed"`
	SnoozedUntil *time.Time         `json:"snoozed_until,omitempty"`
	Dismissible  bool               `json:"dismissible"`
}

// AnnouncementStatus is the feed state returned next to the items.
type AnnouncementStatus struct {
	Mode            string `json:"mode"`
	Sequence        int64  `json:"sequence"`
	FetchedAt       string `json:"fetched_at"`
	VerifiedAt      string `json:"verified_at"`
	LastError       string `json:"last_error"`
	Stale           bool   `json:"stale"`
	TrustConfigured bool   `json:"trust_configured"`
	SourceURL       string `json:"source_url"`
}

// AnnouncementList is the GET /api/v1/announcements response.
type AnnouncementList struct {
	Items  []Announcement     `json:"items"`
	Status AnnouncementStatus `json:"status"`
}

type announcementBanner struct {
	Items []Announcement `json:"items"`
}

// ListAnnouncements calls GET /api/v1/announcements.
func (c *Client) ListAnnouncements(ctx context.Context) (AnnouncementList, error) {
	return getJSON[AnnouncementList](ctx, c, "/api/v1/announcements")
}

// AnnouncementBanner calls GET /api/v1/announcements/banner: unsnoozed critical notices.
func (c *Client) AnnouncementBanner(ctx context.Context) ([]Announcement, error) {
	b, err := getJSON[announcementBanner](ctx, c, "/api/v1/announcements/banner")
	return b.Items, err
}

// MarkAnnouncementRead calls POST /api/v1/announcements/{id}/read.
func (c *Client) MarkAnnouncementRead(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/announcements/"+PathEscape(id)+"/read", nil, nil)
}

// DismissAnnouncement calls POST /api/v1/announcements/{id}/dismiss.
func (c *Client) DismissAnnouncement(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/announcements/"+PathEscape(id)+"/dismiss", nil, nil)
}

// SnoozeAnnouncement calls POST /api/v1/announcements/{id}/snooze; the server caps the duration.
func (c *Client) SnoozeAnnouncement(ctx context.Context, id, duration string) error {
	body := map[string]string{"duration": duration}
	return c.do(ctx, http.MethodPost, "/api/v1/announcements/"+PathEscape(id)+"/snooze", body, nil)
}
