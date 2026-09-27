package scripts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// s3TestServerImageTag matches a pinned, versioned image reference such as
// "adobe/s3mock:3.11.0". It requires a dotted version after the colon, so an
// edit that drops the pin to ":latest" or a bare image name fails this
// check instead of floating silently.
var s3TestServerImageTag = regexp.MustCompile(`\bimage=\S+:\d+\.\d+(\.\d+)?\S*\b`)

// TestAttachmentsMinioScriptPinsImageTag checks that the disposable S3
// server used by the gated S3 blobstore tests names a specific version, not
// "latest" or an untagged image: a floating tag changes what a test run
// exercises with no corresponding diff to review.
func TestAttachmentsMinioScriptPinsImageTag(t *testing.T) {
	script, err := os.ReadFile(filepath.Join(sourceRepoRoot(t), "scripts", "attachments-minio.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !s3TestServerImageTag.Match(script) {
		t.Fatalf("scripts/attachments-minio.sh does not pin a versioned image tag:\n%s", script)
	}
}
