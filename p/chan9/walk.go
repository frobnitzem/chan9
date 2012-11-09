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
		newfid.Clnt = fid.Clnt
		newfid.Type = fid.Type
		newfid.Qid = qid
		newfid.Cname = PathJoin(fid.Cname, wnames)
		newfid.walked = true
	}

	return rc.Wqid, nil
}

/*  Wrapper for fid.Walk to deal with possibility of walking > 16 steps,
    and of running into a mount-point along the way.
 */
func (ns *Namespace) Walk(fid *Fid, wnames []string) (*Fid, error) {
	var err error = nil
	var wqid []p.Qid
	var i int
	var found bool

	newfid := fid.Clnt.FidAlloc()

	for { // step in blocks of 16 path elems
		n := len(wnames)
		if n > 16 {
			n = 16
		}

		wqid, err = fid.Walk(newfid, wnames[0:n])
		if err != nil {
			goto error
		}
		// Check for hitting mount-points and recurse.
		for i = 0; i < len(wqid); i++ {
			c := ns.Mnt.CheckMount(fid.Type, fid.Dev, wqid[i])
			if c == nil {
				continue
			}
			if i == len(wnames)-1 {
				break
			}
			// Walk the next name, testing for errors to find correct union.
			// TODO: move the following to 'WalkUnion'
			// to call from partial walks, e.g. mkdir
			found = false
			for _, fs := range c {
				wqid, err = fs.Walk(newfid, wnames[i+1:i+2])
				if err != nil || len(wqid) != 1 {
					continue
				}
				// Successful mount lookup.
				wnames = wnames[i+1:] // since it gets bumped below.
				i = 0
				n = 1
				found = true
				break
			}
			if !found {
				err = Enofile
				goto error
			}
		}

		if len(wqid) != n {
			err = Enofile
			goto error
		}

		wnames = wnames[n:]
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

// Walks to a named file, using the same algo. as Walk, but always starting
// from ns.Root. Returns a Fid associated with the file, or an Error.
func (ns *Namespace) FWalk(e Elemlist) (*Fid, error) {
	var fid *Fid
	switch e.Ref {
	case '/':
		fid = ns.Root
	case '.':
		fid = ns.WdFid
	default:
		return nil, Enofile
	}
	return ns.Walk(fid, e.Elems)
}

// Starting from the file associated with fid, walks all wnames in
// sequence and associates the resulting file with newfid. If no wnames
// were walked successfully, an Error is returned. Otherwise a slice with a
// Qid for each walked name is returned.
//func (ns *Namespace) Walk(fid *Fid, newfid *Fid, wnames []string) ([]p.Qid, error) {
//	return fid.Walk(newfid, wnames)
//}

