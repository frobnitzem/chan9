// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chan9

import (
	"code.google.com/p/go9p/p"
	"fmt"
	"syscall"
)

/* Starting from the file associated with fid, walks all e.Elems names in
   sequence and associates the resulting file with newfid. If no elems
   were walked successfully, an Error is returned. Otherwise a slice with a
   Qid for each walked name is returned.
   The newfid is only valid if all names are walked, and all
   wnames must be user-searchable directories.
   wnames must also be less than MAXWELEM=16 for most servers,
   but this call is staying close to the network call and doesn't deal with that.
    fid.Walk doesn't traverse mnt-points, ns.Walk does.
 */
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
		newfid.Cname, newfid.Path = PathJoin(fid.Cname, wnames,
				fid.Path, fileid_list(fid.Type,fid.Dev,rc.Wqid))
		newfid.walked = true
	}

	return rc.Wqid, nil
}

func fileid_list(Type uint16, Dev uint32, qids []p.Qid) []FileID {
	path := make([]FileID, len(qids))
	for i,q := range qids {
		path[i] = FileID{Type, Dev, q}
	}
	return path
}

/*  Wrapper for fid.Walk to walk only zero steps, and not
    traverse a mount-point, if one exists.
    mntsem defines the mount semantics
      true  -> copy prev, next from cloned fid
      false -> set prev, next = nil in returned fid
 */
func (fid *Fid) Clone(mntsem bool) (*Fid, error) {
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
	if mntsem {
		newfid.prev = fid.prev
		newfid.next = fid.next
	}

	return newfid, nil
}

func (ns *Namespace) WalkDotDot(fid *Fid) (*Fid, error) {
	if fid == nil {
		return nil, Ebaduse
	}
	l := len(fid.Path)
	fmt.Printf("Walking .. from cname: %v, path: %v\n", fid.Cname, fid.Path)
	if l < 2 {
		return fid.Clone(true)
	}
	pfid := ns.Mnt.CheckParent(fid.Path[l-1], fid.FileID)
	if pfid == nil {
		return fid.WalkOne("..")
	}
	nfid, err := pfid.WalkOne("..")
	if err != nil {
		return nil, err
	}
	nfid.Path = append(nfid.Path[:0], fid.Path[:len(fid.Path)-1]...)
	return nfid, nil
}

/*  Wrapper for fid.Walk to walk only one step, and not
    traverse a mount-point, if one exists.
 */
func (ns *Namespace) WalkOne(fid *Fid, wname string) (*Fid, error) {
	if wname == ".." {
		return ns.WalkDotDot(fid)
	}
	return fid.WalkOne(wname)
}
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
    This routine is required if the fid is user-accessible,
    since Fid.next refers to an internal Mnttable fid.
    That Mnttable fid can't be Clunked unless there's an unmount.

    MStep is considered inefficient.
func (f *Fid) MStep() (*Fid, error) {
	if f == nil {
		return nil, Ebaduse
	}
	fid := f.next
	fid, err := fid.Clone(true)
	if err != nil {
		return nil, err
	}
	f.Clunk()
	return fid, nil
} */

/*  Reset the effects of MStep back to the start of the union.
 */
func (f *Fid) MReset() (*Fid, error) {
	var fid *Fid
	
	if f == nil {
		return nil, Ebaduse
	}
	for fid=f; fid.prev!=nil; fid=fid.prev {
	}
	fid, err := fid.Clone(true)
	if err != nil {
		return nil, err
	}
	f.Clunk()
	return fid, nil
}

/* Repeatedly call fn on all union elements
   until an error is not returned.
   - or return the last error.
   It is assumed that fn modifies fid, so
   the input fid is returned on error
   and the successful fid on success.
 */
func (fid *Fid) MUntil(fn func(*Fid)error) error {
	if fid == nil {
		return Ebaduse
	}
	if fid.next != nil || fid.prev != nil {
		var err error
		var fd *Fid
		var f *Fid
		for f=fid; f != nil; f=f.next {
			fd, err = f.Clone(false)
			if err != nil {
				return err
			}
			err = fn(fd)
			if err == nil {
				break
			}
			fd.Clunk()
		}
		if err != nil {
			return err
		}
		fd.prev = f.prev
		fd.next = f.next
		fid.Clunk()
		*fid = *fd
		return nil
	}
	return fn(fid)
}


/*  Wrapper for fid.Walk to deal with possibility of walking > 16 steps,
    and of running into a mount-point along the way.
    Official Song: `Walk', Foo Fighters
 */
func (ns *Namespace) Walk(fid *Fid, wnames []string) (*Fid, error) {
	var err error = nil
	var wqid []p.Qid
	var i int

	if fid == nil {
		return nil, Ebaduse
	}
	fmt.Printf("Walking %v\n", wnames)
	if len(wnames)>0 && wnames[0] == ".." {
		fid, err = ns.WalkDotDot(fid)
		if err != nil {
			return nil, err
		}
		rfid, err := ns.Walk(fid, wnames[1:])
		fid.Clunk()
		return rfid, err
	}

	newfid := fid.Clnt.FidAlloc()
	path := fid.Path
	fmt.Printf("Starting at path: %v\n", path)

	for { // step in blocks of 16 path elems
		n := len(wnames)
		if n > 16 {
			n = 16
		}

		fid.Type &= ^NOREMAP
		Type := fid.Type
		Dev := fid.Dev
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
			// ¿move the following to 'WalkUnion'?
			// and protect against changing Children[wqid[i]]
			// to call from partial walks, e.g. mkdir
			// - replace unions with a channel to manage state?
			// - replace fid with an int to prevent fid tampering?
			c := ns.Mnt.CheckMount(FileID{fid.Type,fid.Dev,wqid[i]})
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
			path = append(path, fileid_list(Type, Dev, wqid[:n])...)
			wnames = wnames[n:]
			continue // ensures we clone the client fid.
		}

		if len(wqid) != n {
			err = Enofile
			goto error
		}

		path = append(path, fileid_list(Type, Dev, wqid[:n])...)
		wnames = wnames[n:]
		fid = newfid
		if len(wnames) == 0 {
			break
		}
	}
	fmt.Printf("Ending at path: %v\n", path)

	newfid.Path = path
	return newfid, nil

error:
	newfid.Clunk()
	return nil, err
}

/* Walks to a named file, using the same algo. as Walk, but translating
 * Elemlist.  Returns a Fid associated with the file, or an Error.
 * tlast controls whether the last element is traversed if it's a mount-point.
 */
func (ns *Namespace) FWalk(e Elemlist) (*Fid, error) {
	var fid *Fid
	switch e.Ref {
	case '/':
		fid = ns.Root
	case '.':
		fid = ns.Cwd
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
		fid = ns.Cwd
	default:
		return nil, Enofile
	}

	l := len(e.Elems)
	switch l {
	case 0:
		return fid.Clone(false)
	case 1:
		return ns.WalkOne(fid, e.Elems[0])
	}

	fid, err = ns.Walk(fid, e.Elems[:l-1])
	if err != nil {
		return nil, err
	}
	return ns.WalkOne(fid, e.Elems[l-1])
}

// Starting from the file associated with fid, walks all wnames in
// sequence and associates the resulting file with newfid. If no wnames
// were walked successfully, an Error is returned. Otherwise a slice with a
// Qid for each walked name is returned.
//func (ns *Namespace) Walk(fid *Fid, newfid *Fid, wnames []string) ([]p.Qid, error) {
//	return fid.Walk(newfid, wnames)
//}

