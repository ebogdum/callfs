package auth

import "testing"

func TestAdminSetMembership(t *testing.T) {
	s := NewAdminSet([]string{"alice", ""})

	if !s.IsAdmin("alice") {
		t.Errorf("IsAdmin(alice) = false, want true")
	}
	if s.IsAdmin("bob") {
		t.Errorf("IsAdmin(bob) = true, want false")
	}
	if s.IsAdmin("") {
		t.Errorf("IsAdmin(empty) = true, want false")
	}
	if !s.IsAdmin("root") {
		t.Errorf("IsAdmin(root) = false, want true")
	}
}

func TestAdminSetReplace(t *testing.T) {
	s := NewAdminSet([]string{"alice"})

	s.Replace([]string{"bob"})

	if s.IsAdmin("alice") {
		t.Errorf("IsAdmin(alice) after Replace = true, want false")
	}
	if !s.IsAdmin("bob") {
		t.Errorf("IsAdmin(bob) after Replace = false, want true")
	}
	if !s.IsAdmin("root") {
		t.Errorf("IsAdmin(root) after Replace = false, want true")
	}
}

func TestAdminSetNilReceiver(t *testing.T) {
	var s *AdminSet

	if s.IsAdmin("alice") {
		t.Errorf("nil IsAdmin(alice) = true, want false")
	}
	if !s.IsAdmin("root") {
		t.Errorf("nil IsAdmin(root) = false, want true")
	}
}
