//go:build cgo

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func bdAttachment(t *testing.T, bd, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"attachment"}, args...)
	cmd := exec.Command(bd, fullArgs...)
	cmd.Dir = dir
	cmd.Env = bdEnv(dir)
	stdout, stderr, err := runCommandBuffers(t, cmd)
	if err != nil {
		t.Fatalf("bd attachment %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func bdAttachmentFail(t *testing.T, bd, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"attachment"}, args...)
	cmd := exec.Command(bd, fullArgs...)
	cmd.Dir = dir
	cmd.Env = bdEnv(dir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected bd attachment %s to fail, but succeeded:\n%s", strings.Join(args, " "), out)
	}
	return string(out)
}

func TestEmbeddedAttachmentCommands(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}

	bd := buildEmbeddedBD(t)
	dir, beadsDir, _ := bdInit(t, bd, "--prefix", "att")
	issue := bdCreate(t, bd, dir, "Attachment issue", "--type", "task")

	source := filepath.Join(dir, "body.md")
	if err := os.WriteFile(source, []byte("# Attachment\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	addOut := bdAttachment(t, bd, dir, "add", issue.ID, source, "--json")
	var added attachmentListItem
	if err := json.Unmarshal([]byte(addOut), &added); err != nil {
		t.Fatalf("parse attachment add JSON: %v\n%s", err, addOut)
	}
	if added.IssueID != issue.ID {
		t.Fatalf("added issue_id = %q, want %q", added.IssueID, issue.ID)
	}
	if added.OriginalFilename != "body.md" {
		t.Fatalf("original filename = %q", added.OriginalFilename)
	}
	if added.Missing {
		t.Fatal("added attachment is unexpectedly missing")
	}

	storedPath := filepath.Join(beadsDir, filepath.FromSlash(added.StorageRelPath))
	if _, err := os.Stat(storedPath); err != nil {
		t.Fatalf("stored attachment not found at %s: %v", storedPath, err)
	}

	listOut := bdAttachment(t, bd, dir, "list", issue.ID, "--json")
	var listed []attachmentListItem
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("parse attachment list JSON: %v\n%s", err, listOut)
	}
	if len(listed) != 1 || listed[0].ID != added.ID {
		t.Fatalf("listed attachments = %+v, want one %s", listed, added.ID)
	}

	orphanPath := filepath.Join(beadsDir, "attachments", issue.ID, "orphan")
	if err := os.WriteFile(orphanPath, []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsckOut := bdAttachment(t, bd, dir, "fsck", "--json")
	var fsck attachmentMaintenanceResult
	if err := json.Unmarshal([]byte(fsckOut), &fsck); err != nil {
		t.Fatalf("parse attachment fsck JSON: %v\n%s", err, fsckOut)
	}
	if fsck.Count != 1 || fsck.Unreachable[0].StorageRelPath != filepath.ToSlash(filepath.Join("attachments", issue.ID, "orphan")) {
		t.Fatalf("fsck result = %+v, want orphan", fsck)
	}
	bdAttachment(t, bd, dir, "prune", "--dry-run")
	if _, err := os.Stat(orphanPath); err != nil {
		t.Fatalf("dry-run prune removed orphan unexpectedly: %v", err)
	}
	bdAttachment(t, bd, dir, "prune")
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatalf("orphan still exists or stat failed unexpectedly: %v", err)
	}

	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyOut := bdAttachment(t, bd, dir, "copy", issue.ID, added.ShortHash, outDir, "--json")
	var copied map[string]interface{}
	if err := json.Unmarshal([]byte(copyOut), &copied); err != nil {
		t.Fatalf("parse attachment copy JSON: %v\n%s", err, copyOut)
	}
	copiedPath := filepath.Join(outDir, "body.md")
	data, err := os.ReadFile(copiedPath)
	if err != nil {
		t.Fatalf("copy target missing: %v", err)
	}
	if string(data) != "# Attachment\n" {
		t.Fatalf("copied data = %q", data)
	}

	failOut := bdAttachmentFail(t, bd, dir, "copy", issue.ID, "body.md", outDir)
	if !strings.Contains(failOut, "already exists") {
		t.Fatalf("overwrite failure = %q, want already exists", failOut)
	}
	bdAttachment(t, bd, dir, "copy", issue.ID, "body.md", outDir, "--force")

	removeOut := bdAttachment(t, bd, dir, "remove", issue.ID, added.ContentHash, "--json")
	var removed map[string]interface{}
	if err := json.Unmarshal([]byte(removeOut), &removed); err != nil {
		t.Fatalf("parse attachment remove JSON: %v\n%s", err, removeOut)
	}
	if removed["status"] != "removed" {
		t.Fatalf("remove status = %v", removed["status"])
	}
	if _, err := os.Stat(storedPath); !os.IsNotExist(err) {
		t.Fatalf("stored attachment still exists or stat failed unexpectedly: %v", err)
	}

	listOut = bdAttachment(t, bd, dir, "list", issue.ID, "--json")
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("parse final attachment list JSON: %v\n%s", err, listOut)
	}
	if len(listed) != 0 {
		t.Fatalf("final listed attachments = %+v, want empty", listed)
	}
}

// TestEmbeddedAttachmentAddDuplicateReturnsFriendlyError covers re-attaching
// the same file to the same issue: the metadata insert hits
// uniq_attachments_issue_hash, and the CLI must surface which attachment
// already holds that content rather than a raw MySQL 1062 error.
func TestEmbeddedAttachmentAddDuplicateReturnsFriendlyError(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}

	bd := buildEmbeddedBD(t)
	dir, _, _ := bdInit(t, bd, "--prefix", "att")
	issue := bdCreate(t, bd, dir, "Duplicate attachment issue", "--type", "task")

	source := filepath.Join(dir, "body.md")
	if err := os.WriteFile(source, []byte("# Attachment\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	addOut := bdAttachment(t, bd, dir, "add", issue.ID, source, "--json")
	var added attachmentListItem
	if err := json.Unmarshal([]byte(addOut), &added); err != nil {
		t.Fatalf("parse attachment add JSON: %v\n%s", err, addOut)
	}

	failOut := bdAttachmentFail(t, bd, dir, "add", issue.ID, source)
	if !strings.Contains(failOut, "already attached as "+added.ID) {
		t.Fatalf("duplicate add output = %q, want it to name %s", failOut, added.ID)
	}
	if strings.Contains(failOut, "1062") || strings.Contains(failOut, "Duplicate entry") {
		t.Fatalf("duplicate add output leaked a raw UNIQUE-key error: %q", failOut)
	}
}

// TestEmbeddedAttachmentRemoveWarnsOnFileRemovalFailure covers the case
// where the metadata delete succeeds but the stored file cannot be removed
// (permission denied on its directory here). The command must still report
// success and drop the metadata row, rather than leaving a dangling
// attachment record because the disk cleanup failed.
func TestEmbeddedAttachmentRemoveWarnsOnFileRemovalFailure(t *testing.T) {
	if os.Getenv("BEADS_TEST_EMBEDDED_DOLT") != "1" {
		t.Skip("set BEADS_TEST_EMBEDDED_DOLT=1 to run embedded dolt integration tests")
	}

	bd := buildEmbeddedBD(t)
	dir, beadsDir, _ := bdInit(t, bd, "--prefix", "att")
	issue := bdCreate(t, bd, dir, "Undeletable attachment issue", "--type", "task")

	source := filepath.Join(dir, "body.md")
	if err := os.WriteFile(source, []byte("# Attachment\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	addOut := bdAttachment(t, bd, dir, "add", issue.ID, source, "--json")
	var added attachmentListItem
	if err := json.Unmarshal([]byte(addOut), &added); err != nil {
		t.Fatalf("parse attachment add JSON: %v\n%s", err, addOut)
	}

	issueAttachmentDir := filepath.Join(beadsDir, "attachments", issue.ID)
	if err := os.Chmod(issueAttachmentDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(issueAttachmentDir, 0o755) })

	removeOut := bdAttachment(t, bd, dir, "remove", issue.ID, added.ContentHash, "--json")
	var removed map[string]interface{}
	if err := json.Unmarshal([]byte(removeOut), &removed); err != nil {
		t.Fatalf("parse attachment remove JSON: %v\n%s", err, removeOut)
	}
	if removed["status"] != "removed" {
		t.Fatalf("remove status = %v, want removed despite the stray file", removed["status"])
	}
	if removed["file_removed"] != false {
		t.Fatalf("file_removed = %v, want false", removed["file_removed"])
	}
	if removed["file_remove_error"] == nil || removed["file_remove_error"] == "" {
		t.Fatalf("file_remove_error missing from %+v", removed)
	}

	if err := os.Chmod(issueAttachmentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	listOut := bdAttachment(t, bd, dir, "list", issue.ID, "--json")
	var listed []attachmentListItem
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("parse attachment list JSON: %v\n%s", err, listOut)
	}
	if len(listed) != 0 {
		t.Fatalf("listed attachments after removal = %+v, want empty despite stray file", listed)
	}
}
