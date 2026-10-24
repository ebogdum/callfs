package natsrv

import "testing"

func TestDeriveCredentialsDeterministic(t *testing.T) {
	first, err := DeriveCredentials("s1")
	if nil != err {
		t.Fatalf("first derive failed: %v", err)
	}
	second, err := DeriveCredentials("s1")
	if nil != err {
		t.Fatalf("second derive failed: %v", err)
	}
	if first.UserPublic != second.UserPublic {
		t.Errorf("UserPublic differs: %q != %q", first.UserPublic, second.UserPublic)
	}
	if first.RoutePassword != second.RoutePassword {
		t.Errorf("RoutePassword differs: %q != %q", first.RoutePassword, second.RoutePassword)
	}
}

func TestDeriveCredentialsDiffersBySecret(t *testing.T) {
	one, err := DeriveCredentials("s1")
	if nil != err {
		t.Fatalf("derive s1 failed: %v", err)
	}
	two, err := DeriveCredentials("s2")
	if nil != err {
		t.Fatalf("derive s2 failed: %v", err)
	}
	if one.UserPublic == two.UserPublic {
		t.Errorf("UserPublic equal for different secrets: %q", one.UserPublic)
	}
	if one.RoutePassword == two.RoutePassword {
		t.Errorf("RoutePassword equal for different secrets: %q", one.RoutePassword)
	}
}

func TestDeriveCredentialsEmptySecret(t *testing.T) {
	if _, err := DeriveCredentials(""); nil == err {
		t.Fatal("expected error for empty secret")
	}
}
