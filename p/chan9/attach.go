// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chan9

import (
	"code.google.com/p/go9p/p"
	"net"
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
	proto, netaddr, e := p.ParseNetName(addr)
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
	clnt.Id = addr

	return clnt, nil
}

