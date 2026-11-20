package api

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func TestBackupTargetTestErrorMessage(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   string
	}{
		{"forbidden names the IAM actions", http.StatusForbidden, "s3:ListBucket"},
		{"wrong region", http.StatusMovedPermanently, "another region"},
		{"bad request is a region hint", http.StatusBadRequest, "check the region"},
		{"missing bucket", http.StatusNotFound, "bucket not found"},
		{"other status falls through", http.StatusInternalServerError, "could not reach backup target"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &smithyhttp.ResponseError{
				Response: &smithyhttp.Response{Response: &http.Response{StatusCode: tc.status}},
				Err:      errors.New("boom"),
			}
			if got := backupTargetTestErrorMessage(err); !strings.Contains(got, tc.want) {
				t.Fatalf("message = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}
