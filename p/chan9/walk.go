// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chan9

import (
	"code.google.com/p/go9p/p"
	"syscall"
)

// Starting from the file associated with fid, walks all e.Elems names in
// sequence and associates the resulting file with newfid. If no elems
// were walked successfully, an Error is returned. Otherwise a slice with a
// Qid for each walked name is returned.
// The newfid is only valid if all names are walked, and all
// wnames must be user-searchable directories.
// wnames must also be less than MAXWELEM=16 for most servers,
// but this call is staying close to the network call and doesn't deal with that.
func (fid *Fid) Walk(newfid *Fid, wnames []string) ([]p.Qid, error) {
	tc := fid.Clnt.NewFcall()

	err := p.PackTwalk(tc, fid.Fid, newfid.Fid, wnames)
	if err != nil {
		return nil, err
	}

	rc, err := fid.Clnt.Rpc(tc)
	if err != nil {
		return nil, err
	}
	if rc.Type == p.Rerror {
		return nil, &p.Error{rc.Error, syscall.Errno(rc.Errornum)}
	}

	if len(rc.Wqid) == len(wnames) { // success.
		var qid p.Qid
		if l := len(rc.Wqid); l > 0 {
			qid = rc.Wqid[l-1]
		} else {
			qid = fid.Qid
		}
		newfid.Qid = qid // it should.
		newfid.Cname = PathJoin(fid.Cname, wnames)
		newfid.walked = true
	}

	return rc.Wqid, nil
}

// Walks to a named file, using the same algo. as Walk, but always starting
// from Clnt.Root. Returns a Fid associated with the file, or an Error.
func (clnt *Clnt) FWalk(wnames []string) (*Fid, error) {
	var err error = nil
	var wqid []p.Qid

	newfid := clnt.FidAlloc()
	fid := clnt.Root

	for { // step in blocks of 16 path elems
		n := len(wnames)
		if n > 16 {
			n = 16
		}

		wqid, err = fid.Walk(newfid, wnames[0:n])
		if err != nil {
			goto error
		}
		if len(wqid) != n {
			err = &p.Error{"file not found", p.ENOENT}
			goto error
		}

		wnames = wnames[n:len(wnames)]
		fid = newfid
		if len(wnames) == 0 {
			break
		}
	}

	return newfid, nil

error:
	newfid.Clunk()
	return nil, err
}

// Starting from the file associated with fid, walks all wnames in
// sequence and associates the resulting file with newfid. If no wnames
// were walked successfully, an Error is returned. Otherwise a slice with a
// Qid for each walked name is returned.
//func (ns *Namespace) Walk(fid *Fid, newfid *Fid, wnames []string) ([]p.Qid, error) {
//	return fid.Walk(newfid, wnames)
//}
func (ns *Namespace) FWalk(e Elemlist) (*Fid, error) {
	var wnames []string = e.Elems
	if e.Ref != '/' {
		wnames = PathJoin(ns.Cwd, e.Elems)
		//e.Elems = PathJoin(ns.Cwd, e.Elems)
		//e.Ref = '/'
	}
	mh, mpath := mhead_split(ns.Root, wnames)

	return mh.c.FWalk(mpath)
}

