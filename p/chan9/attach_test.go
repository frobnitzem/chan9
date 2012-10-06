package chan9

import "testing"

func Test_parse_net_name(t *testing.T) {
	cmp_parse := func(addr, anet, anetaddr string) {
		net, netaddr, e := parse_net_name(addr)
		if e != nil {
			t.Errorf("Error parsing \"%s\": %s\n", addr,e.Error())
		}
		if net != anet || netaddr != anetaddr {
			t.Errorf("Invalid parse \"%s\": %s %s\n", addr, net, netaddr)
		}
	}
	ck_err := func(addr string) {
		_, _, e := parse_net_name(addr)
		if e == nil {
			t.Errorf("Parse on \"%s\" did not throw error!\n", addr)
		}
	}
	cmp_parse("tcp!192.168.0.0", "tcp", "192.168.0.0")
	cmp_parse("unix!/tmp/test", "unix", "/tmp/test")
	cmp_parse("192.169.0.0", "tcp", "192.169.0.0")
	cmp_parse("tcp!192.169.0.0!ssh", "tcp", "192.169.0.0:22")
	cmp_parse("tcp6!12::F3::15:0", "tcp6", "12::F3::15:0")
	ck_err("")
	ck_err("!!!")
	ck_err("a!!")
}
