// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package chan9

import "io"
import "code.google.com/p/go9p/p"
import "syscall"

// Reads count bytes starting from offset from the file associated with the fid.
// Returns a slice with the data read, if the operation was successful, or an
// Error.
func (fid *Fid) Read(offset uint64, count uint32) ([]byte, error) {
	if count > fid.Iounit {
		count = fid.Iounit
	}

	tc := fid.Clnt.NewFcall()
	err := p.PackTread(tc, fid.Fid, offset, count)
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

	return rc.Data, nil
}

// Reads up to len(buf) bytes from the File. Returns the number
// of bytes read, or an Error.
func (file *File) Read(buf []byte) (int, error) {
	n, err := file.ReadAt(buf, int64(file.Offset))
	if err == nil {
		file.Offset += uint64(n)
	}

	return n, err
}

// Reads up to len(buf) bytes from the file starting from offset.
// Returns the number of bytes read, or an Error.
func (file *File) ReadAt(buf []byte, offset int64) (int, error) {
	b, err := file.Fid.Read(uint64(offset), uint32(len(buf)))
	if err != nil {
		return 0, err
	}

	if len(b) == 0 {
		return 0, io.EOF
	}

	copy(buf, b)
	return len(b), nil
}

// Reads exactly len(buf) bytes from the File starting from offset.
// Returns the number of bytes read (could be less than len(buf) if
// end-of-file is reached), or an Error.
func (file *File) Readn(buf []byte, offset uint64) (int, error) {
	ret := 0
	for len(buf) > 0 {
		n, err := file.ReadAt(buf, int64(offset))
		if err != nil {
			return 0, err
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

/* TODO: make seek rewind dir-s
func (file *File) Seek(off int) error {
	if file.Fid.prev != nil {
		file.MReset()
	}
} */

// Reads the content of the directory associated with the File.
// Returns an array of maximum num entries (if num is 0, returns
// all entries from the directory). If the operation fails, returns
// an Error.
func (file *File) Readdir(num int) ([]*p.Dir, error) {
	buf := make([]byte, file.Fid.Clnt.Msize-p.IOHDRSZ)
	dirs := make([]*p.Dir, 32)
	pos := 0
	for {
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return nil, err
		}

		if n == 0 {
			if file.Fid.next == nil {
				break
			}
			fid, err := file.Fid.next.Clone(true)
			if err != nil {
				return nil, err
			}
			err = fid.Open(p.OREAD)
			if err != nil {
				fid.Clunk()
				return nil, err
			}
			file.Fid.Clunk()
			file.Fid = fid
			file.Offset = 0
			if cap(buf) < int(fid.Clnt.Msize-p.IOHDRSZ) {
				buf = make([]byte, fid.Clnt.Msize-p.IOHDRSZ)
			} else {
				buf = buf[:fid.Clnt.Msize-p.IOHDRSZ]
			}
			continue
		}

		for b := buf[0:n]; len(b) > 0; {
			d, perr := p.UnpackDir(b, file.Fid.Clnt.Dotu)
			if perr != nil {
				return nil, perr
			}

			b = b[d.Size+2 : len(b)]
			if pos >= len(dirs) {
				s := make([]*p.Dir, len(dirs)+32)
				copy(s, dirs)
				dirs = s
			}

			dirs[pos] = d
			pos++
			if num != 0 && pos >= num {
				break
			}
		}
	}
	return dirs[0:pos], nil
}
