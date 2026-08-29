package test

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// auditActions returns a count of audit_logs rows grouped by action.
func auditActions(t *testing.T) map[string]int {
	t.Helper()
	db, err := sql.Open("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("open audit db: %v", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT action, count(*) FROM audit_logs GROUP BY action")
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var action string
		var n int
		if err := rows.Scan(&action, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[action] = n
	}
	return out
}

func TestAudit_RecordsSecurityEvents(t *testing.T) {
	e := newApp(t)

	// Register owner + a member; login the owner; transfer ownership.
	owner, _ := register(t, e, "audit_owner")
	member, memberID := register(t, e, "audit_member")
	wid := createWedding(t, e, owner, "Audit Wedding")
	joinAs(t, e, owner, wid, "editor", member)

	// Login generates a login_success audit event.
	if code, _ := do(t, e, "POST", "/api/v1/auth/login", "", map[string]any{
		"username": "audit_owner", "password": "password123",
	}); code != 200 {
		t.Fatalf("login: want 200, got %d", code)
	}

	// Transfer ownership generates an ownership_transferred event.
	if code, _ := do(t, e, "POST", "/api/v1/weddings/"+wid+"/transfer", owner, map[string]any{
		"user_id": memberID,
	}); code != 200 {
		t.Fatalf("transfer: want 200, got %d", code)
	}

	got := auditActions(t)
	if got["login_success"] < 1 {
		t.Fatalf("expected a login_success audit event, got %v", got)
	}
	if got["ownership_transferred"] < 1 {
		t.Fatalf("expected an ownership_transferred audit event, got %v", got)
	}
}
