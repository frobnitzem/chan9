package chan9

import "testing"

func TestParsename(t *testing.T) {
	var c int
	alleq := func (e Elemlist, isdir bool, n ...string) bool {
		if isdir != e.Mustbedir || len(e.Elem) != len(n) {
			return true
		}
		for i := range(n) {
			if n[i] != e.Elem[i] {
				return true
			}
		}
		return false
	}
	testpath := func (p string, isdir bool, n ...string) int {
		e := Parsename(p)
		if alleq(e, isdir, n...) {
			t.Errorf("Invalid parse\"%s\": %#v\n", p, e)
			return 1
		}
		return 0
	}
	c += testpath("../test/../../1/././/.", true, "..", "..", "1")
	c += testpath("../test/../../1", false, "..", "..", "1")
	c += testpath("/../test/./1/a/..//../4/5/./", true, "test", "4", "5")
	c += testpath("/../test/./1/a/..//../4/5", false, "test", "4", "5")
	c += testpath("/test/1/2/../", true, "test", "1")
	c += testpath("/test/1/2/..", true, "test", "1")
	c += testpath("/./../1//a/2/../syntax./", true, "1", "a", "syntax.")
	c += testpath("/./../1//a/2/../syntax.", false, "1", "a", "syntax.")
	c += testpath("/test/../../../12/../.././../..////../..", true, ".")
	c += testpath("/test/../../../...", false, "...")
	if c > 0 {
		t.Errorf("Failed %d tests!", c)
		return
	}
	t.Log("Parsename passed!")
}

