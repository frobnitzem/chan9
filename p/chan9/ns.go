/*
 * Copyright 2009 The Go Authors.  All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 *
 * Copyright 2012 David M. Rogers
 *
 */

package chan9

import (
	"code.google.com/p/go9p/p"
	//"os"
	"sync"
)

// The top-level namespace keeps track
// of the mounted p9 clients and the user's fid-s.
type Namespace struct {
	sync.Mutex
	//Cwd      []string
	Mnt	   *Mnttab
	Root	   *Fid
	Cwd        *Fid
	//Root       *NSElem // This one must be of type NSMOUNT, else there is no
			   //    server to accept 9p messages.
	fidpool    *pool
	err        error
}

var Enofile error = &p.Error{"file not found", p.ENOENT}
var Ebaduse error = &p.Error{"bad use of fid", p.EINVAL}

/* Initializes a namespace object from a client.
 * It calls Mount to do the initial attachment,
 * which respects Clnt.Subpath.
 */
func NSFromClnt(c *Clnt, afd *Fid, flags uint32, aname string) (*Namespace, error) {
	fid, err := c.Attach(afd, c.User, aname)
	if err != nil {
		return nil, err
	}

	ns := new(Namespace)
	//ns.User = c.User // p.OsUsers.Uid2User(os.Geteuid())
	ns.fidpool = c.fidpool // newPool(p.NOFID)
	ns.Mnt = NewMnttab(c.Dev) // Puts Dev in the Mnttab,
	ns.Root = fid // so it doesn't flip when we mount.
	ns.Cwd, err = ns.Walk(ns.Root, make([]string,0))
	if err != nil {
		return nil, err
	}

	return ns, nil
}

func (ns *Namespace) Cd(dir string) error {
	e := ParseName(dir)
	fid, err := ns.FWalk(e)
	if err != nil {
		return err
	}
	ns.Cwd.Clunk()
	ns.Cwd = fid
	return nil
}

/* Clone a namespace object, uses copy-on-write semantics.
   TODO: currently incorrectly implemented
 */
func (ons *Namespace) clone() (*Namespace, error) {
	ns := new(Namespace)

	ons.Lock()
	ns.Root = ons.Root
	//ns.Cwd = make([]string, len(ons.Cwd))
	//copy(ns.Cwd, ons.Cwd)
        ns.fidpool = newPool(p.NOFID) // should walk(0) all fids
	ons.Unlock()

	clnts.Lock() // should manage clnts more effectively
	clnts.Unlock()

	return ns, nil
}

func (ns *Namespace) Close() {
	// Not implemented
	return
}

const NOREMAP uint16 = 1<<15 // or-ed into Type field to ensure it's not the parent of a mnt

// A Fid type represents a file on the server. Fids are used for the
// low level methods that correspond directly to the 9P2000 message requests
type Fid struct {
	sync.Mutex
	Clnt   *Clnt // Client the fid belongs to
	Cname	[]string // server!/subpath/path != Plan9
	Path    []FileID // list of id-s ~ Plan9 Cname
	Iounit uint32
	FileID
	/*Type uint16   // Channel type (index of function call table) -- FYI
			// left-most bit indicates 'non-remappable' type/dev pair
	Dev uint32    // Server or device number distinguishing the server from others of the same type
			// duplicates Clnt * info
	Qid p.Qid     // The Qid description for the file - direct inclusion conflicts, shadowing Type
	*/
	Mode   uint8  // Open mode (one of p.O* values) (if file is open)
	Fid    uint32 // Fid number
	p.User        // The user the fid belongs to
	walked bool   // true if the fid points to a walked file on the server
	// options for representing union dir-s
	prev   *Fid
	next   *Fid
	MayCreate bool
	MayCache  bool
}

// The file is similar to the Fid, but is used in the high-level client
// interface.
type File struct {
	Fid    *Fid
	Offset uint64
}

type pool struct {
	sync.Mutex
	need  int
	nchan chan uint32
	maxid uint32
	imap  []byte
}

