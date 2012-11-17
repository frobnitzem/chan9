// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chan9

import (
	"code.google.com/p/go9p/p"
	"syscall"
)

// Opens the file associated with the fid. Returns nil if
// the operation is successful.
func (fid *Fid) Open(mode uint8) error {
	if fid == nil {
		return Ebaduse
	}
	if fid.next != nil || fid.prev != nil {
		fn := func(f *Fid) error {
			return f.Open(mode)
		}
		return fid.MUntil(fn)
	}
	tc := fid.Clnt.NewFcall()
	err := p.PackTopen(tc, fid.Fid, mode)
	if err != nil {
		return err
	}

	rc, err := fid.Clnt.Rpc(tc)
	if err != nil {
		return err
	}
	if rc.Type == p.Rerror {
		return &p.Error{rc.Error, syscall.Errno(rc.Errornum)}
	}

	fid.Qid = rc.Qid
	fid.Iounit = rc.Iounit
	if fid.Iounit == 0 || fid.Iounit > fid.Clnt.Msize-p.IOHDRSZ {
		fid.Iounit = fid.Clnt.Msize - p.IOHDRSZ
	}
	fid.Mode = mode
	return nil
}

// Creates a file in the directory associated with the fid. Returns nil
// if the operation is successful.
func (fid *Fid) Create(name string, perm uint32, mode uint8, ext string) error {
	if fid == nil {
		return Ebaduse
	}
	if fid.prev != nil || fid.next != nil { // union
		f := fid
		for ; f.next != nil; f=f.next {
			if f.MayCreate {
				break
			}
		}
		if !f.MayCreate {
			return &p.Error{"No writable directory in union", p.ENOENT}
		}
		nf, err := f.Clone(false)
		if err != nil {
			return err
		}
		err = nf.Create(name, perm, mode, ext)
		if err != nil {
			nf.Clunk()
			return err
		}
		fid.Clunk()
		*fid = *nf
		return nil
	}
	tc := fid.Clnt.NewFcall()
	err := p.PackTcreate(tc, fid.Fid, name, perm, mode, ext, fid.Clnt.Dotu)
	if err != nil {
		return err
	}

	rc, err := fid.Clnt.Rpc(tc)
	if err != nil {
		return err
	}
	if rc.Type == p.Rerror {
		return &p.Error{rc.Error, syscall.Errno(rc.Errornum)}
	}

	fid.Qid = rc.Qid
	fid.Iounit = rc.Iounit
	if fid.Iounit == 0 || fid.Iounit > fid.Clnt.Msize-p.IOHDRSZ {
		fid.Iounit = fid.Clnt.Msize - p.IOHDRSZ
	}
	fid.Mode = mode
	return nil
}

// Creates and opens a named file.
// Returns the file if the operation is successful, or an Error.
func (ns *Namespace) FCreate(e Elemlist, perm uint32, mode uint8) (*File, error) {
	n := len(e.Elems)-1
	if n < 0 || e.Elems[n] == ".." {
		return nil, &p.Error{"invalid path", p.ENOENT}
	}
	if e.Mustbedir {
		perm = perm | p.DMDIR
	}
	name := e.Elems[n]
	e.Elems = e.Elems[:n]

	fid, err := ns.FWalk(e)
	if err != nil {
		return nil, err
	}

	err = fid.Create(name, perm, mode, "")
	if err != nil {
		fid.Clunk()
		return nil, err
	}

	return &File{fid, 0}, nil
}

// Opens a named file. Returns the opened file, or an Error.
func (ns *Namespace) FOpen(path Elemlist, mode uint8) (*File, error) {
	fid, err := ns.FWalk(path)
	if err != nil {
		return nil, err
	}

	err = fid.Open(mode)
	if err != nil {
		fid.Clunk()
		return nil, err
	}

	return &File{fid, 0}, nil
}
