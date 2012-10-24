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
	Cwd        []string
	Root       *NSElem // This one must be of type NSMOUNT, else there is no
			   //    server to accept 9p messages.
	fidpool    *pool
	err        error
}

/* Initializes a namespace object from a client.
 * It calls Mount to do the initial attachment,
 * which respects Clnt.Subpath.
 */
func NSFromClnt(c *Clnt, afd *Fid, flags uint32, aname string) (*Namespace, error) {
	ns := new(Namespace)
	//ns.User = c.User // p.OsUsers.Uid2User(os.Geteuid())
        ns.fidpool = c.fidpool // newPool(p.NOFID)

	ns.Root = new(NSElem)
	ns.Root.Etype = NSPASS
	ns.Root.Cname = make([]string, 0)
	ns.Root.MayCreate = flags & p.DMWRITE != 0 // TODO: check. flags option to Mount.
	ns.Root.Child = make(map[string]*NSElem)
	ns.Root.c = c
	ns.Root.Parent = make([]*NSElem, 1)
	ns.Root.Parent[0] = ns.Root
	ns.Cwd = make([]string, 0)

	err := ns.Mount(c, afd, "/", flags, aname)
	if err != nil {
		return nil, err
	}

	return ns, nil
}

/* Clone a namespace object, uses copy-on-write semantics.
 */
func (ons *Namespace) Clone() (*Namespace, error) {
	ns := new(Namespace)

	ons.Lock()
	ns.Root = ons.Root
	ns.Cwd = make([]string, len(ons.Cwd))
	copy(ns.Cwd, ons.Cwd)
        ns.fidpool = newPool(p.NOFID) // FIXME -- don't drop existing fid-s
	ons.Unlock()

	clnts.Lock() // FIXME -- manage clnts more effectively
	for _,c := range clnts.c {
		c.incref()
	}
	clnts.Unlock()

	return ns, nil
}

func (ns *Namespace) Close() {
	return
}

// A Fid type represents a file on the server. Fids are used for the
// low level methods that correspond directly to the 9P2000 message requests
type Fid struct {
	sync.Mutex
	Clnt   *Clnt // Client the fid belongs to
	Cname	[]string
	Iounit uint32
	//Type uint16   // Channel type (index of function call table) -- FYI
	Dev uint32    // Server or device number distinguishing the server from others of the same type
			// duplicates Clnt * info
	p.Qid         // The Qid description for the file
	Mode   uint8  // Open mode (one of p.O* values) (if file is open)
	Fid    uint32 // Fid number
	p.User        // The user the fid belongs to
	walked bool   // true if the fid points to a walked file on the server
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

