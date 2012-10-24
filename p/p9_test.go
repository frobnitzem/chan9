package p

import "testing"

func TestParseNetName(t *testing.T) {
	var c int
	cmp_parse := func(addr, anet, anetaddr string) int {
		net, netaddr, e := ParseNetName(addr)
		if e != nil {
			t.Errorf("Error parsing \"%s\": %s\n", addr,e.Error())
			return 1
		}
		if net != anet || netaddr != anetaddr {
			t.Errorf("Invalid parse \"%s\": %s %s\n", addr, net, netaddr)
			return 1
		}
		return 0
	}
	ck_err := func(addr string) int {
		_, _, e := ParseNetName(addr)
		if e == nil {
			t.Errorf("Parse on \"%s\" did not throw error!\n", addr)
			return 1
		}
		return 0
	}
	c += cmp_parse("tcp!192.168.0.0", "tcp", "192.168.0.0")
	c += cmp_parse("unix!/tmp/test", "unix", "/tmp/test")
	c += cmp_parse("192.169.0.0", "tcp", "192.169.0.0")
	c += cmp_parse("tcp!192.169.0.0!ssh", "tcp", "192.169.0.0:22")
	c += cmp_parse("tcp6!12::F3::15:0", "tcp6", "12::F3::15:0")
	c += ck_err("!!!")
	c += ck_err("a!!")
	if c == 0 {
		t.Log("ParseName passed!")
	}
}
