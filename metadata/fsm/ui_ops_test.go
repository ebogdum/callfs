package fsm

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ebogdum/callfs/metadata"
)

func seedUILinks(t *testing.T, f *Machine, base time.Time) {
	t.Helper()
	applyCmd(t, f, Command{Op: "create_link", Link: &metadata.SingleUseLink{
		Token: "tok-a", FilePath: "/a.txt", Owner: "api-user-0", Status: "active",
		ExpiresAt: base.Add(time.Hour), CreatedAt: base,
	}})
	applyCmd(t, f, Command{Op: "create_link", Link: &metadata.SingleUseLink{
		Token: "tok-b", FilePath: "/b.txt", Owner: "api-user-1", Status: "active",
		ExpiresAt: base.Add(-time.Minute), CreatedAt: base.Add(time.Second),
	}})
	applyCmd(t, f, Command{Op: "create_link", Link: &metadata.SingleUseLink{
		Token: "tok-c", FilePath: "/c.txt", Owner: "api-user-0", Status: "used",
		ExpiresAt: base.Add(time.Hour), CreatedAt: base.Add(2 * time.Second),
	}})
}

func TestUIRevokeLinkMarksActiveLinkRevoked(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	stamp := base.Add(time.Minute)
	f := New()
	seedUILinks(t, f, base)

	res := applyCmd(t, f, Command{Op: "revoke_link", LinkID: LinkIDForToken("tok-a"), Timestamp: stamp})
	if "" != res.Err {
		t.Fatalf("revoke returned error %q", res.Err)
	}
	link := f.state.LinksByToken["tok-a"]
	if "revoked" != link.Status {
		t.Fatalf("status = %q, want revoked", link.Status)
	}
	if !link.UpdatedAt.Equal(stamp) {
		t.Fatalf("UpdatedAt = %v, want leader stamp %v", link.UpdatedAt, stamp)
	}
}

func TestUIRevokeLinkIsDeterministicAcrossReplicas(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	stamp := base.Add(time.Minute)
	first := New()
	second := New()
	seedUILinks(t, first, base)
	seedUILinks(t, second, base)

	cmd := Command{Op: "revoke_link", LinkID: LinkIDForToken("tok-a"), Timestamp: stamp}
	applyCmd(t, first, cmd)
	applyCmd(t, second, cmd)

	firstState, err := json.Marshal(first.state)
	if nil != err {
		t.Fatalf("marshal first state: %v", err)
	}
	secondState, err := json.Marshal(second.state)
	if nil != err {
		t.Fatalf("marshal second state: %v", err)
	}
	if string(firstState) != string(secondState) {
		t.Fatalf("replicas diverged after revoke_link")
	}
}

func TestUIRevokeLinkRejectsNotActiveAndUnknown(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := New()
	seedUILinks(t, f, base)

	if res := applyCmd(t, f, Command{Op: "revoke_link", LinkID: LinkIDForToken("tok-c"), Timestamp: base}); "precondition_failed" != res.Err {
		t.Fatalf("revoke of used link err = %q, want precondition_failed", res.Err)
	}
	if res := applyCmd(t, f, Command{Op: "revoke_link", LinkID: LinkIDForToken("missing"), Timestamp: base}); "not_found" != res.Err {
		t.Fatalf("revoke of unknown link err = %q, want not_found", res.Err)
	}
}

func TestUIGetLinkByIDReturnsDerivedID(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := New()
	seedUILinks(t, f, base)

	link, ok := f.linkByID(LinkIDForToken("tok-a"), base)
	if !ok {
		t.Fatalf("linkByID did not find tok-a")
	}
	if LinkIDForToken("tok-a") != link.ID || "active" != link.Status {
		t.Fatalf("unexpected link %+v", link)
	}
	if _, ok := f.linkByID(LinkIDForToken("missing"), base); ok {
		t.Fatalf("linkByID found an unknown id")
	}
}
