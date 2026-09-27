package issueops

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestRejectPersistenceDemotionRefusesIssueWithAttachment covers a demotion
// path (bd update --ephemeral / --no-history) that carries the target row
// over to the wisp plane by inserting it there and then issuing DELETE FROM
// issues. copyPersistenceAuxiliary only copies persistenceAuxTables, which
// does not include attachments, so an issue with an attachment row would
// have that row (and the bytes it names on disk) cascade-deleted silently
// unless the demotion is refused first.
func TestRejectPersistenceDemotionRefusesIssueWithAttachment(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM issue_snapshots WHERE issue_id = ?`)).
		WithArgs("bd-abc").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM compaction_snapshots WHERE issue_id = ?`)).
		WithArgs("bd-abc").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM attachments WHERE issue_id = ?`)).
		WithArgs("bd-abc").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err = rejectPersistenceDemotion(ctx, tx, "bd-abc")
	if err == nil {
		t.Fatal("rejectPersistenceDemotion() = nil, want error for an issue with an attachment")
	}
	if !strings.Contains(err.Error(), "attachments") {
		t.Fatalf("rejectPersistenceDemotion() error = %q, want it to mention attachments", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// TestRejectPersistenceDemotionAllowsIssueWithoutRetainedRows is the
// counterpart: an issue with no snapshots and no attachments demotes freely.
func TestRejectPersistenceDemotionAllowsIssueWithoutRetainedRows(t *testing.T) {
	ctx := context.Background()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer tx.Rollback()

	for _, table := range []string{"issue_snapshots", "compaction_snapshots", "attachments"} {
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM `+table+` WHERE issue_id = ?`)).
			WithArgs("bd-abc").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	}

	if err := rejectPersistenceDemotion(ctx, tx, "bd-abc"); err != nil {
		t.Fatalf("rejectPersistenceDemotion() error = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
