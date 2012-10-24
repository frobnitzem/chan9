package chan9

import (
	"code.google.com/p/go9p/p"
)


// The real mount command.
// -- see http://plan9.bell-labs.com/sys/doc/lexnames.html (but note Cnames are here
//                      as immutable path-names rooted (=[]) at the nearest mount point)
//    and http://man.cat-v.org/inferno/2/sys-bind
//    The client should start with ref=1.
//    This routine takes over management of the Client (unless it returns with error)
//    so call Clnt.incref() if you need to keep it.
func (ns *Namespace) Mount(clnt *Clnt, afd *Fid, oldloc string, flags uint32, aname string) error {
	if flags > p.MMASK-1 {
		return &p.Error{"bad mount flags", p.EINVAL}
	}
	fid, err := clnt.Attach(afd, clnt.User, aname)
	if err != nil {
		return err
	}
	// walk the client to reach clnt.Subpath
	if len(clnt.Subpath) > 0 {
		Qids, err := fid.Walk(fid, clnt.Subpath)
		if err != nil {
			return err
		}
		if len(Qids) != len(clnt.Subpath) {
			return &p.Error{"subpath not found on client", p.ENOENT}
		}
	}

	oldroot := clnt.Root
	oldfidpool := clnt.fidpool
	clnt.Root = fid
	clnt.fidpool = ns.fidpool

	e := ns.RootPath(oldloc) // get the rooted path
	loc, err := ns.Extend(ns.Root, e.Elems)

	if err != nil {
		clnt.Root = oldroot
		clnt.fidpool = oldfidpool
		return err
	}

	l := NewNSElem(loc.Cname...)
	l.Etype = NSMOUNT
	l.c = clnt
	clnt.incref()
	l.MayCreate = flags&p.MCREATE != 0
	l.MayCache = flags&p.MCACHE != 0
	ns.replace(loc, l, flags)
	return nil
}

func NewNSElem(Cname ...string) *NSElem {
	l := new(NSElem)
	l.Etype = NSPASS
	l.Cname = Cname
	l.Parent = make([]*NSElem, 0)
	l.Child = make(map[string]*NSElem)
	return l
}

/* Replace an NSElem with the info. from a new one.  The replacement
   ensures that links point to the newly updated location.
 */
func (ns *Namespace) replace(loc, e *NSElem, flags uint32) {
	op := func() {
		ns._replace(loc, e, flags)
	}
	ns.gc(op)
}

func (ns *Namespace) _replace(loc, e *NSElem, flags uint32) {
	if flags&p.MORDER == p.MREPL { // don't need the client pointer in any case.
		ns.unlink(loc)
		loc.Etype = e.Etype
		loc.c = e.c
		loc.Cname = e.Cname
		//loc.Parent = e.Parent // inherits parents
		loc.Child = e.Child
		loc.MayCreate = e.MayCreate
		loc.MayCache = e.MayCache
	} else { // dir becomes union
		switch loc.Etype {
		case NSPASS:
			fallthrough
		case NSMOUNT: // cast to union
			nelem := ns.Birth(loc, "")
			nelem.Etype = loc.Etype
			nelem.c = loc.c
			loc.Etype = NSUNION
			loc.c = nil
			loc.u = make([]*NSElem, 2)[:1]
			loc.u[0] = e
			fallthrough
		case NSUNION:
			// check if clnt is already in loc.u ?
			switch flags&p.MORDER {
			case p.MAFTER:
				loc.u = append(loc.u, e)
			case p.MBEFORE:
				l := len(loc.u)
				if cap(loc.u) < l+1 {
					loc.u = make([]*NSElem, l+3)[:l+1]
				} else {
					loc.u = loc.u[:l+1]
				}
				copy(loc.u[1:l+1], loc.u[:l])
				loc.u[0] = e
			}
		}
	}

	return
}

/* Extends the namespace by adding NSElems where they don't exist,
   verifying at each step that the path exists.  If it doesn't,
   the ns is cleaned up to match, and an error is returned.
   It breaks up the task by recursion.
 */
func (ns *Namespace) Extend(mhead *NSElem, dirs []string) (end *NSElem, err error) {
	var i int
	loc := mhead
	next := loc

	if len(dirs) == 0 { // corner case, important for bootstrapping
		return mhead, nil
	}
	for i = 0; i < len(dirs); i++ {
		next = loc.Lookup(dirs[i])
		if next == nil || next.Etype != NSPASS {
			break
		}
		loc = next
	}

	// Trust, but verify.
	newfid := mhead.c.FidAlloc()
	qids, err := mhead.c.Root.Walk(newfid, PathJoin(mhead.Cname, dirs[:i]))
	if err != nil {
		return nil, err
	}
	newfid.Clunk()
	if j := i+len(mhead.Cname)-len(qids); j != 0 {
		if j == 1 { // found nselem, but couldn't walk it
			ns.remove(next)
			return nil, &p.Error{"Remote dir removed", p.ENOENT}
		} else {
			return ns.Extend(next, dirs[:i+1-j])
		}
	}

	if i == len(dirs) {
		return loc, nil
	}
	for _,p := range dirs[i:] {
		loc = ns.Birth(loc, p)
	}
	return ns.Extend(next, dirs[i:])
}

// Mount's cousin (to, from), since we re-direct "from (=newloc)" to "to (=oldloc)".
// Note, however, that the arguments are in the opposite order compared to ln -s.
func (ns *Namespace) Bind(oldloc, newloc string, flags uint32) error {
	if flags > p.MMASK-1 {
		return &p.Error{"bad bind flags", p.EINVAL}
	}
	op := ns.RootPath(oldloc)
	np := ns.RootPath(newloc)
	// walk both locations
	ons, err := ns.Extend(ns.Root, op.Elems)
	if err != nil {
		return err
	}
	nns, err := ns.Extend(ns.Root, np.Elems)
	if err != nil {
		return err
	}
	
	ons.MayCreate = flags&p.MCREATE != 0 // FIXME: must deal with these attrs differently
	ons.MayCache = flags&p.MCACHE != 0
	ns.replace(nns, ons, flags)
	
	return nil
}

/* Returns a list of things pointing here and things here points at (if a mount/union).
 */
func (ns *Namespace) LsMounts(path string) ([][]string, [][]string, error) {
	parents := make([][]string, 0)
	children := make([][]string, 0)

	e := ns.RootPath(path)
	loc, err := ns.Extend(ns.Root, e.Elems)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range loc.Parent {
		if p.Etype == NSUNION {
			parents = append(parents, p.Cname) // TODO: this only gives the name up to its NSMOUNT
		}
	}
	switch loc.Etype {
	case NSMOUNT:
		children = append(children, loc.Cname) // TODO: give a string descr. of the client
	case NSUNION:
		for _, c := range loc.u {
			children = append(children, c.Cname) // TODO: this only gives the name up to its NSMOUNT
		}
	}
	return parents, children, nil
}

func (ns *Namespace) Unmount(oldloc, newloc string) error {
	//var oper func()

	op := ns.RootPath(oldloc)
	np := ns.RootPath(newloc)
	// walk both locations
	ons, err := ns.Extend(ns.Root, op.Elems)
	if err != nil {
		return err
	}
	nns, err := ns.Extend(ns.Root, np.Elems)
	if err != nil {
		return err
	}
	
	switch nns.Etype { // TODO
	case NSMOUNT:
	case NSUNION:
	default:
		return &p.Error{"invalid mount point", p.ENOENT}
	}
	ons.Etype = ons.Etype
	
	return nil
}

