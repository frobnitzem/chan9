// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chan9

import (
	"code.google.com/p/go9p/p"
	"strings"
	"syscall"
)

// Opens the file associated with the fid. Returns nil if
// the operation is successful.
func (fid *Fid) Open(mode uint8) error {
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
func (ns *Namespace) FCreate(path string, perm uint32, mode uint8) (*File, error) {
	n := strings.LastIndex(path, "/")
	if n < 0 {
		n = 0
	}

	fid, err := ns.FWalk(path[0:n])
	if err != nil {
		return nil, err
	}

	if path[n] == '/' {
		n++
	}

	err = fid.Create(path[n:], perm, mode, "")
	if err != nil {
		fid.Clunk()
		return nil, err
	}

	return &File{fid, 0}, nil
}

// Opens a named file. Returns the opened file, or an Error.
func (ns *Namespace) FOpen(path string, mode uint8) (*File, error) {
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
