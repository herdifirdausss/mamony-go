//go:build integration

package identity

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// TestIdentityMailgunNoticeMonitorRole exercises the actual v18
// read-only credential and trusted public facade on disposable PostgreSQL.
// No live webhook, email provider or production database is contacted.
func TestIdentityMailgunNoticeMonitorRole(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	monitorDSN := guardedDB(t, "TEST_IDENTITY_NOTICE_MONITOR_URL",
		"mamony_identity_notice_monitor_runtime")
	got, err := ReadSecurityNoticeHealthAsMonitor(ctx, monitorDSN)
	if err != nil || got.PendingDue < 0 || got.Dead < 0 ||
		got.SubmittedUnknown < 0 || got.Delivered < 0 ||
		got.ProviderFailures < 0 || got.UnmatchedEvents < 0 {
		t.Fatalf("restricted notice health facade returned invalid aggregate: %+v %v", got, err)
	}
	// An observer may not impersonate maintenance or erase queued notices.
	if _, err := ReadSecurityNoticeHealth(ctx, monitorDSN); err == nil {
		t.Fatal("monitor credential was accepted as maintenance/runtime credential")
	}
	monitor, err := pgx.Connect(ctx, monitorDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = monitor.Close(context.Background()) }()
	for _, sql := range []string{
		"SELECT mamony_identity.purge_old_mailgun_security_receipts()",
		"SELECT mamony_identity.purge_old_password_change_notices()",
	} {
		if _, err := monitor.Exec(ctx, sql); err == nil {
			t.Fatalf("read-only monitor can invoke destructive maintenance: %s", sql)
		}
	}
}
