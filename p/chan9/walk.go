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
	if fid == nil {
		return nil, Ebaduse
	}
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

/*  Wrapper for fid.Walk to walk only zero steps, and not
    traverse a mount-point, if one exists.
 */
func (fid *Fid) Clone() (*Fid, error) {
	var wnames = []string{}

	if fid == nil {
		return nil, Ebaduse
	}
	newfid := fid.Clnt.FidAlloc()

	_, err := fid.Walk(newfid, wnames)
	if err != nil {
		newfid.Clunk()
		return nil, err
	}

	return newfid, nil
}

/*  Wrapper for fid.Walk to walk only one step, and not
    traverse a mount-point, if one exists.
 */
func (fid *Fid) WalkOne(wname string) (*Fid, error) {
	var wnames = []string { wname }

	if fid == nil {
		return nil, Ebaduse
	}
	newfid := fid.Clnt.FidAlloc()

	wqid, err := fid.Walk(newfid, wnames)
	if err != nil || len(wqid) != 1 {
		if fid.next != nil { // Unionized.
			newfid.Clunk()
			return fid.next.WalkOne(wname)
		}
		if err == nil {
			err = Enofile
		}
		goto error
	}

	return newfid, nil

error:
	newfid.Clunk()
	return nil, err
}

/*  Step to the next element of a linked-list, union fid.
    It will always clunk the input (unless it's nil).
    This routine is required if the fid is user-accessible,
    since Fid.next refers to an internal Mnttable fid.
    That Mnttable fid can't be Clunked unless there's an unmount.
 */
func (f *Fid) MStep() (*Fid, error) {
	var wnames = []string{}
	if f == nil {
		return nil, Ebaduse
	}
	fid := f.next
	f.Clunk()
	if fid == nil {
		return nil, nil
	}
	newfid := fid.Clnt.FidAlloc()
	_, err := fid.Walk(newfid, wnames)
	if err != nil {
		newfid.Clunk()
		return nil, err
	}
	newfid.prev = fid.prev
	newfid.next = fid.next
	return newfid, nil
}

/*  Wrapper for fid.Walk to deal with possibility of walking > 16 steps,
    and of running into a mount-point along the way.
 */
func (ns *Namespace) Walk(fid *Fid, wnames []string) (*Fid, error) {
	var err error = nil
	var wqid []p.Qid
	var i int

	if fid == nil {
		return nil, Ebaduse
	}
	newfid := fid.Clnt.FidAlloc()

	for { // step in blocks of 16 path elems
		n := len(wnames)
		if n > 16 {
			n = 16
		}

		wqid, err = fid.Walk(newfid, wnames[0:n])
		if err != nil || (n > 0 && len(wqid) == 0) {
			if fid.next != nil { // Unionized.
				newfid.Clunk()
				fid = fid.next
				newfid = fid.Clnt.FidAlloc()
				continue
			}
			goto error
		}
		// Check for hitting mount-points and recurse.
		if len(wnames) == 0 { // copy union semantics from self
			newfid.next = fid.next
			newfid.prev = fid.prev
		}
		for i = 0; i < len(wqid); i++ {
			// ¿TODO?: move the following to 'WalkUnion'
			// and protect against changing Children[wqid[i]]
			// to call from partial walks, e.g. mkdir
			// - replace unions with a channel to manage state?
			// - replace fid with an int to prevent fid tampering?
			c := ns.Mnt.CheckMount(fid.Type, fid.Dev, wqid[i])
			if c == nil {
				// TODO: Check for symlinks and dial 'em!
				continue
			}
			fid = c
			newfid.Clunk() // the fid churn is to satisfy incref/decref
			newfid = fid.Clnt.FidAlloc()
			break
		}
		if i < len(wqid) { // replaced by a mount point
			n = i+1
			wnames = wnames[n:]
			continue // ensures we clone the client fid.
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

/* Walks to a named file, using the same algo. as Walk, but always starting
 * from ns.Root. Returns a Fid associated with the file, or an Error.
 * tlast controls whether the last element is traversed if it's a mount-point.
 */
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

/* Walks to a named file, but does not traverse the last element
 * if it's a mount-point.
 */
func (ns *Namespace) FWalkTo(e Elemlist) (*Fid, error) {
	var err error
	var fid *Fid
	switch e.Ref {
	case '/':
		fid = ns.Root
	case '.':
		fid = ns.WdFid
	default:
		return nil, Enofile
	}

	l := len(e.Elems)
	switch l {
	case 0:
		return fid.Clone()
	case 1:
		return fid.WalkOne(e.Elems[0])
	}

	fid, err = ns.Walk(fid, e.Elems[:l-1])
	if err != nil {
		return nil, err
	}
	return fid.WalkOne(e.Elems[l-1])
}

// Starting from the file associated with fid, walks all wnames in
// sequence and associates the resulting file with newfid. If no wnames
// were walked successfully, an Error is returned. Otherwise a slice with a
// Qid for each walked name is returned.
//func (ns *Namespace) Walk(fid *Fid, newfid *Fid, wnames []string) ([]p.Qid, error) {
//	return fid.Walk(newfid, wnames)
//}

