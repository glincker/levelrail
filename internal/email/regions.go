package email

import (
	"fmt"
	"slices"
)

// SESRegions is the one registry of AWS regions that offer SES; the API validates against it and the UI mirrors it.
var SESRegions = []string{
	"us-east-1", "us-east-2", "us-west-1", "us-west-2",
	"af-south-1",
	"ap-south-1", "ap-south-2", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3",
	"ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-southeast-5",
	"ca-central-1", "ca-west-1",
	"eu-central-1", "eu-central-2", "eu-west-1", "eu-west-2", "eu-west-3",
	"eu-north-1", "eu-south-1", "eu-south-2",
	"il-central-1", "me-south-1", "me-central-1", "mx-central-1", "sa-east-1",
	"us-gov-west-1",
}

// ValidSESRegion reports whether region is a listed SES region.
func ValidSESRegion(region string) bool {
	return slices.Contains(SESRegions, region)
}

// SESSMTPHost derives the SES SMTP endpoint for region.
func SESSMTPHost(region string) (string, error) {
	if !ValidSESRegion(region) {
		return "", fmt.Errorf("email: %q is not a supported SES region", region)
	}
	return "email-smtp." + region + ".amazonaws.com", nil
}
