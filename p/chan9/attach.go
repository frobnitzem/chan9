// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chan9

import (
	"code.google.com/p/go9p/p"
	"fmt"
	"net"
	"strings"
	"syscall"
)

// Creates an authentication fid for the specified user. Returns the fid, if
// successful, or an Error.
func (clnt *Clnt) Auth(user p.User, aname string) (*Fid, error) {
	fid := clnt.FidAlloc()
	tc := clnt.NewFcall()
	err := p.PackTauth(tc, fid.Fid, user.Name(), aname, uint32(user.Id()), clnt.Dotu)
	if err != nil {
		return nil, err
	}

	_, err = clnt.Rpc(tc)
	if err != nil {
		return nil, err
	}

	fid.walked = true
	return fid, nil
}

// Creates a fid for the specified user that points to the root
// of the file server's file tree. Returns a Fid pointing to the root,
// if successful, or an Error.
func (clnt *Clnt) Attach(afid *Fid, user p.User, aname string) (*Fid, error) {
	var afno uint32

	if afid != nil {
		afno = afid.Fid
	} else {
		afno = p.NOFID
	}

	fid := clnt.FidAlloc()
	tc := clnt.NewFcall()
	err := p.PackTattach(tc, fid.Fid, afno, user.Name(), aname, uint32(user.Id()), clnt.Dotu)
	if err != nil {
		return nil, err
	}

	rc, err := clnt.Rpc(tc)
	if err != nil {
		return nil, err
	}
	if rc.Type == p.Rerror {
		return nil, &p.Error{rc.Error, syscall.Errno(rc.Errornum)}
	}

	fid.Qid = rc.Qid
	fid.Cname = fid.Cname[:0]
	fid.walked = true
	return fid, nil
}

// Dial a server and return a non-attached client "channel."
func Dial(addr string) (*Clnt, error) {
	proto, netaddr, e := parse_net_name(addr)
	if e != nil {
		return nil, &p.Error{e.Error(), p.EIO}
	}
	c, e := net.Dial(proto, netaddr)
	if e != nil {
		return nil, &p.Error{e.Error(), p.EIO}
	}

	clnt, err := Connect(c, 8192+p.IOHDRSZ, true)
	if err != nil {
		return nil, err
	}

	return clnt, nil
}

/* From http://swtch.com/plan9port/man/man3/dial.html
   addr is a network address of the form network!netaddr!service,
   network!netaddr, or simply netaddr. Network is tcp, udp, unix,
   or the special token, net. Net is a free variable that stands
   for any network in common between the source and the host netaddr.
   Netaddr can be a host name, a domain name, or a network address. 

   -- we should subtract the "net" option and add, from the Dial package:
    Known networks are "tcp", "tcp4" (IPv4-only), "tcp6" (IPv6-only), "udp",
    "udp4" (IPv4-only), "udp6" (IPv6-only), "ip", "ip4" (IPv4-only),
    "ip6" (IPv6-only), "unix" and "unixpacket".

    For TCP and UDP networks, addresses have the form host:port.
    If host is a literal IPv6 address, it must be enclosed in square brackets.
    The functions JoinHostPort and SplitHostPort manipulate addresses in this form. 
 */
func parse_net_name(addr string) (string, string, error) {
	var proto string
	var a []string
	var netaddr string

	a = strings.Split(addr, "!")
	if l := len(a); l == 0 || l > 3 {
		return "", "", &p.Error{"unable to parse name", p.EINVAL}
	}
	if len(a) >= 2 {
		proto = a[0]
		a = a[1:]
	}
	netaddr = a[0]
	if netaddr == "" {
		return "", "", &p.Error{"unable to parse name", p.EINVAL}
	}
	if proto == "" { // Detect network type
		/*if strings.Count(netaddr, "." == 3) && (
			for i,v := range(strings.Split(netaddr, ".") {
				if _, ok := strconv.Atoi(); !ok {
					break
				}
			})
		} */
		proto = "tcp" // or just guess
	}
	
	if len(a) == 2 {
		port, e := net.LookupPort(proto, a[1])
		if e != nil {
			return "", "", e
		}
		netaddr = net.JoinHostPort(netaddr, fmt.Sprintf("%d", port))
	}
	return proto, netaddr, nil
}

