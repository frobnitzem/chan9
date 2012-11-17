package chan9

import "testing"
import "code.google.com/p/go9p/p"

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
	palleq := func (a, b []FileID) bool {
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
	testwalk := func (a, b, d []string, pa, pb, pd []FileID) int {
		c, pc  := PathJoin(a, b, pa, pb)
		if ! alleq(c, d) {
			t.Errorf("Invalid PathJoin %v, %v -> %v\n", a, b, c)
			return 1
		}
		if ! palleq(pc, pd) {
			t.Errorf("Invalid PathJoin %v, %v -> %v\n", pa, pb, pc)
			return 1
		}
		return 0
	}
	s := func (a ...string) []string {
		return a
	}
	q := func (a ...FileID) []FileID {
		return a
	}
	f1 := FileID{0,1,p.Qid{0,1,2}}
	f2 := FileID{1,1,p.Qid{0,1,2}}
	f3 := FileID{0,1,p.Qid{1,1,0}}
	f4 := FileID{0,1,p.Qid{0,1,0}}

	c += testwalk(s(), s("ra"), s("ra"),
			q(), q(f1), q(f1))
	c += testwalk(s("ra", "wr"), s(), s("ra", "wr"),
			q(f1, f2), q(), q(f1, f2))
	c += testwalk(s("a", "ha", "r"), s("..", "..", "c"), s("a", "c"),
			q(f1, f2, f2), q(f2, f1, f3), q(f1, f3))
	c += testwalk(s("a", "ha", "r"), s("c"),
			s("a", "ha", "r", "c"), q(f1, f2, f3), q(f4),
			q(f1, f2, f3, f4))
	c += testwalk(s("start", "ha", "r"), s("..", "2", "c"),
			s("start", "ha", "2", "c"),
			q(f1, f2, f1), q(f4, f3, f4), q(f1, f2, f3, f4))
	c += testwalk(s("..", "r"), s("bling", "..", "c"),
			s("..", "r", "bling", "..", "c"),
			q(f4, f2), q(f1, f4, f3), q(f4, f2, f1, f4, f3))
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

