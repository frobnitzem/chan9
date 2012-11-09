// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chan9

import "code.google.com/p/go9p/p"

// Clunks a fid. Returns nil if successful.
func (fid *Fid) Clunk() (err error) {
	err = nil
	if fid.walked {
		tc := fid.Clnt.NewFcall()
		err := p.PackTclunk(tc, fid.Fid)
		if err != nil {
			return err
		}

		_, err = fid.Clnt.Rpc(tc)
	}

	fid.Clnt.fidpool.putId(fid.Fid)
	fid.Clnt.decref()
	fid.walked = false
	fid.Fid = p.NOFID
	return
}

// Closes a file. Returns nil if successful.
func (file *File) Close() error {
	// Should we cancel all pending requests for the File
	return file.Fid.Clunk()
}
