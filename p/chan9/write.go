// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chan9

import "code.google.com/p/go9p/p"
import "syscall"

// Write up to len(data) bytes starting from offset. Returns the
// number of bytes written, or an Error.
func (fid *Fid) Write(data []byte, offset uint64) (int, error) {
	if uint32(len(data)) > fid.Iounit {
		data = data[0:fid.Iounit]
	}

	tc := fid.Clnt.NewFcall()
	err := p.PackTwrite(tc, fid.Fid, offset, uint32(len(data)), data)
	if err != nil {
		return 0, err
	}

	rc, err := fid.Clnt.Rpc(tc)
	if err != nil {
		return 0, err
	}
	if rc.Type == p.Rerror {
		return 0, &p.Error{rc.Error, syscall.Errno(rc.Errornum)}
	}

	return int(rc.Count), nil
}

// Writes up to len(buf) bytes to a file. Returns the number of
// bytes written, or an Error.
func (file *File) Write(buf []byte) (int, error) {
	n, err := file.WriteAt(buf, int64(file.Offset))
	if err == nil {
		file.Offset += uint64(n)
	}

	return n, err
}

// Writes up to len(buf) bytes starting from offset. Returns the number
// of bytes written, or an Error.
func (file *File) WriteAt(buf []byte, offset int64) (int, error) {
	return file.Fid.Write(buf, uint64(offset))
}

// Writes exactly len(buf) bytes starting from offset. Returns the number of
// bytes written. If Error is returned the number of bytes can be less
// than len(buf).
func (file *File) Writen(buf []byte, offset uint64) (int, error) {
	ret := 0
	for len(buf) > 0 {
		n, err := file.WriteAt(buf, int64(offset))
		if err != nil {
			return ret, err
		}

		if n == 0 {
			break
		}

		buf = buf[n:len(buf)]
		offset += uint64(n)
		ret += n
	}

	return ret, nil
}
