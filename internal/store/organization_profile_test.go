package store

import (
	"context"
	"errors"
	"testing"
)

func TestOrganizationProfile_SlugUniqueAndAvatar(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, id := range []string{"org_a", "org_b"} {
		if err := db.SaveOrganization(ctx, Organization{ID: id, Name: id, CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
			t.Fatal(err)
		}
	}
	slug, other := "acme-co", "ACME-CO"
	if _, err := db.UpdateOrganizationProfile(ctx, "org_a", OrgProfilePatch{Slug: &slug}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpdateOrganizationProfile(ctx, "org_b", OrgProfilePatch{Slug: &other}); !errors.Is(err, ErrOrgSlugTaken) {
		t.Errorf("duplicate slug error = %v, want ErrOrgSlugTaken", err)
	}
	def := "team-x"
	if _, err := db.UpdateOrganizationProfile(ctx, DefaultTeamID, OrgProfilePatch{Slug: &def}); err == nil {
		t.Error("the default organization accepted a slug")
	}
	if err := db.SetOrganizationAvatar(ctx, "org_a", OrgAvatar{Mime: "image/png", Bytes: []byte{1}, SHA: "abc"}); err != nil {
		t.Fatal(err)
	}
	if o, _ := db.GetOrganization(ctx, "org_a"); o.AvatarSHA != "abc" {
		t.Errorf("avatar sha = %q", o.AvatarSHA)
	}
	if err := db.DeleteOrganizationAvatar(ctx, "org_a"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteOrganizationAvatar(ctx, "org_a"); !errors.Is(err, ErrOrgAvatarNotFound) {
		t.Errorf("second delete = %v", err)
	}
}
