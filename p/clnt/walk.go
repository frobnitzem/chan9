// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package clnt

import (
	"code.google.com/p/go9p/p"
	"strings"
	"syscall"
)

// Starting from the file associated with fid, walks all wnames in
// sequence and associates the resulting file with newfid. If no wnames
// were walked successfully, an Error is returned. Otherwise a slice with a
// Qid for each walked name is returned.
func (clnt *Clnt) Walk(fid *Fid, newfid *Fid, wnames []string) ([]p.Qid, error) {
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
		newfid.Qid = qid
		newfid.walked = true
	}

	return rc.Wqid, nil
}

// Walks to a named file. Returns a Fid associated with the file,
// or an Error.
func (clnt *Clnt) FWalk(path string) (*Fid, error) {
	var err error = nil
	var wqid []p.Qid

	var i, m int
	for i = 0; i < len(path); i++ {
		if path[i] != '/' {
			break
		}
	}

	if i > 0 {
		path = path[i:len(path)]
	}

	wnames := strings.Split(path, "/")
	newfid := clnt.FidAlloc()
	fid := clnt.Root
	newfid.User = fid.User

	/* get rid of the empty names */
	for i, m = 0, 0; i < len(wnames); i++ {
		if wnames[i] != "" {
			wnames[m] = wnames[i]
			m++
		}
	}

	wnames = wnames[0:m]
	for {
		n := len(wnames)
		if n > 16 {
			n = 16
		}

		wqid, err = clnt.Walk(fid, newfid, wnames[0:n])
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
	clnt.Clunk(newfid)
	return nil, err
}
