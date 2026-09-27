package providers

import "testing"

func TestParseRepository(t *testing.T) {
	got, err := ParseRepository("https://gitlab.com/konflux-qe/dr_test_mathwizz_gl.git")
	if err != nil {
		t.Fatal(err)
	}
	if got.Owner != "konflux-qe" || got.Name != "dr_test_mathwizz_gl" {
		t.Fatalf("unexpected repository: %#v", got)
	}
}

func TestValidateExpectedSHARejectsStaleVersion(t *testing.T) {
	if err := ValidateExpectedSHA("old", "new"); err == nil {
		t.Fatal("expected stale-version error")
	}
}
