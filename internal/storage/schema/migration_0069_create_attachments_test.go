package schema

import "testing"

// TestLatestVersionIncludesMigration0069 pins the real next free slot this
// migration claims, superseding 0068's own version of this test
// (LatestVersion() moved from 68 to 69 the moment this migration file was
// added). Deliberately a hardcoded literal for the same reason 0068's was:
// LatestVersion() drifting to 69 for the wrong reason (an unrelated
// migration landing first) should still be caught by this test failing to
// explain why 69 is attachments-shaped.
func TestLatestVersionIncludesMigration0069(t *testing.T) {
	const want = 69
	if got := LatestVersion(); got != want {
		t.Fatalf("LatestVersion() = %d, want %d (attachments table migration slot claimed by bd-t8j5.1)", got, want)
	}
}
