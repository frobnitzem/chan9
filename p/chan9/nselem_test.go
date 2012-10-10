package chan9

import "testing"

func Test_update_cname(t *testing.T) {
	var c int
	alleq := func (a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range(a) {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	testwalk := func (a, b, d []string) int {
		c := PathJoin(a, b)
		if ! alleq(c, d) {
			t.Errorf("Invalid PathJoin %v, %v -> %v\n", a, b, c)
			return 1
		}
		return 0
	}
	s := func (a ...string) []string {
		return a
	}

	c += testwalk(s(), s("ra"), s("ra"))
	c += testwalk(s("ra", "wr"), s(), s("ra", "wr"))
	c += testwalk(s("a", "ha", "r"), s("..", "..", "c"), s("a", "c"))
	c += testwalk(s("a", "ha", "r"), s("c"),
			s("a", "ha", "r", "c"))
	c += testwalk(s("start", "ha", "r"), s("..", "2", "c"),
			s("start", "ha", "2", "c"))
	c += testwalk(s("..", "r"), s("bling", "..", "c"),
			s("..", "r", "bling", "..", "c"))
	if c > 0 {
		t.Errorf("Failed %d tests!", c)
		return
	}
	t.Log("PathJoin passed!")
}

func TestParseName(t *testing.T) {
	var c int
	alleq := func (e Elemlist, ref rune, isdir bool, n ...string) bool {
		if ref != e.Ref || isdir != e.Mustbedir || len(e.Elems) != len(n) {
			return true
		}
		for i := range(n) {
			if n[i] != e.Elems[i] {
				return true
			}
		}
		return false
	}
	testpath := func (p string, ref rune, isdir bool, n ...string) int {
		e := ParseName(p)
		if alleq(e, ref, isdir, n...) {
			t.Errorf("Invalid parse\"%s\": %#v\n", p, e)
			return 1
		}
		return 0
	}
	c += testpath("blob/bleh/blarf", '.', false, "blob", "bleh", "blarf")
	c += testpath("/", '/', true)
	c += testpath("../test/../../1/././/.", '.', true, "..", "..", "1")
	c += testpath("../test/../../1", '.', false, "..", "..", "1")
	c += testpath("/../test/./1/a/..//../4/5/./", '/', true, "test", "4", "5")
	c += testpath("/../test/./1/a/..//../4/5", '/', false, "test", "4", "5")
	c += testpath("/test/1/2/../", '/', true, "test", "1")
	c += testpath("/test/1/2/..", '/', true, "test", "1")
	c += testpath("/./../1//a/2/../syntax./", '/', true, "1", "a", "syntax.")
	c += testpath("/./../1//a/2/../syntax.", '/', false, "1", "a", "syntax.")
	c += testpath("/test/../../../12/../.././../..////../..", '/', true)
	c += testpath("/test/../../../...", '/', false, "...")
	if c > 0 {
		t.Errorf("Failed %d tests!", c)
		return
	}
	t.Log("ParseName passed!")
}

