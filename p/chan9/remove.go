// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chan9

import "code.google.com/p/go9p/p"
import "syscall"

// Removes the file associated with the Fid. Returns nil if the
// operation is successful.
func (fid *Fid) Remove() error {
	tc := fid.Clnt.NewFcall()
	err := p.PackTremove(tc, fid.Fid)
	if err != nil {
		return err
	}

	rc, err := fid.Clnt.Rpc(tc)
	fid.Clnt.fidpool.putId(fid.Fid)
	fid.Fid = p.NOFID

	if rc.Type == p.Rerror {
		return &p.Error{rc.Error, syscall.Errno(rc.Errornum)}
	}

	return err
}

// Removes the named file. Returns nil if the operation is successful.
func (ns *Namespace) FRemove(path string) error {
	var err error
	fid, err := ns.FWalk(path)
	if err != nil {
		return err
	}

	err = fid.Remove()
	return err
}
