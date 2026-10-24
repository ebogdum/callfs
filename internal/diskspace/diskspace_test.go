package diskspace

import "testing"

func TestOfReportsTheTempDirectory(t *testing.T) {
	usage, err := Of(t.TempDir())
	if nil != err {
		t.Fatal(err)
	}
	if 0 == usage.Total || usage.Free > usage.Total {
		t.Fatalf("unexpected usage %+v", usage)
	}
}
