package models

import "testing"

func TestRoleRankOrdering(t *testing.T) {
	if RoleOwner.Rank() <= RoleEditor.Rank() || RoleEditor.Rank() <= RoleViewer.Rank() {
		t.Fatalf("rank order wrong: owner=%d editor=%d viewer=%d",
			RoleOwner.Rank(), RoleEditor.Rank(), RoleViewer.Rank())
	}
	if Role("bogus").Rank() != 0 {
		t.Fatal("unknown role should rank 0")
	}
}

func TestRoleValid(t *testing.T) {
	for _, r := range []Role{RoleOwner, RoleEditor, RoleViewer} {
		if !r.Valid() {
			t.Fatalf("%s should be valid", r)
		}
	}
	if Role("bogus").Valid() {
		t.Fatal("bogus role should be invalid")
	}
}
