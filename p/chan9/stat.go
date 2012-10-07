// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chan9

import "code.google.com/p/go9p/p"
import "syscall"

// Returns the metadata for the file associated with the Fid, or an Error.
func (fid *Fid) Stat() (*p.Dir, error) {
	tc := fid.Clnt.NewFcall()
	err := p.PackTstat(tc, fid.Fid)
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

	return &rc.Dir, nil
}

// Returns the metadata for a named file, or an Error.
func (ns *Namespace) FStat(path string) (*p.Dir, error) {
	fid, err := ns.FWalk(path)
	if err != nil {
		return nil, err
	}

	d, err := fid.Stat()
	fid.Clunk()
	return d, err
}

// Modifies the data of the file associated with the Fid, or an Error.
func (fid *Fid) Wstat(dir *p.Dir) error {
	tc := fid.Clnt.NewFcall()
	err := p.PackTwstat(tc, fid.Fid, dir, fid.Clnt.Dotu)
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

	return nil
}
