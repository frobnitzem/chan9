package chan9

import (
	"code.google.com/p/go9p/p"
)


const (
	MREPL = (1 << iota)
	MAFTER
	MBEFORE
	MCREATE
)

// The real mount command.
// -- see http://plan9.bell-labs.com/sys/doc/lexnames.html (but note Cnames don't entirely work
//                      here, since dir-s have immutable Cnames assoc. with their creation path)
//    and http://man.cat-v.org/inferno/2/sys-bind
//    This routine takes over management of the Client, (unless it returns with error)
//    so call Clnt.incref() if you need to keep it.
func (ns *Namespace) Mount(clnt *Clnt, afd *Fid, oldloc string, flags uint32, aname string) error {
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

	myfid, err := ns.FWalk(ParseName(oldloc)) // ensure that 'from' exists and is a dir.
	if err != nil {
		clnt.Root = oldroot
		clnt.fidpool = oldfidpool
		return err
	}
	dirs := myfid.Cname // get the rooted path
	myfid.Clunk()

	// attach client to correct location in ns.
	loc := ns.Root
	var i int
	for i = 0; i < len(dirs); i++ {
		next := loc.Lookup(dirs[i])
		if next == nil {
			break
		}
		loc = next
	}
	/*if i == len(dirs) {
		TODO: close garbage connections past "dirs" (careful on Union)
	}*/
	for _,p := range dirs[i:] {
		loc = loc.Birth(p, nil)
	}
	// TODO: deal with MBEFORE/MAFTER union creation flags.
	loc.Etype = NSMOUNT
	loc.c = clnt

	return nil
}

// Mount's cousin (to, from), since we re-direct from (newloc) to to (oldloc).
// Note, however, that the arguments are in the opposite order compared to ln -s.
func (ns *Namespace) Bind(oldloc, newloc string, flags uint32) error {
	return nil
}

func (ns *Namespace) Unmount(oldloc, newloc string) error {
	return nil
}

