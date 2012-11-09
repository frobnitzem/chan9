package chan9

import (
	"code.google.com/p/go9p/p"
	"strings"
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

	e := ParseName(oldloc)
	parent, err := ns.FWalk(e) // walk to fid on parent side

	if err != nil {
		clnt.Root = oldroot
		clnt.fidpool = oldfidpool
		return err
	}
	err = ns.Mnt.Mount(fid, clnt.Root, flags)
	if err != nil {
		clnt.Root = oldroot
		clnt.fidpool = oldfidpool
		parent.Clunk()
		return err
	}

	return nil
}

// Mount's cousin (to, from), since we re-direct "from (=newloc)" to "to (=oldloc)".
// Note, however, that the arguments are in the opposite order compared to ln -s.
func (ns *Namespace) Bind(oldloc, newloc string, flags uint32) error {
	if flags > p.MMASK-1 {
		return &p.Error{"bad bind flags", p.EINVAL}
	}
	ppath := ParseName(newloc)
	cpath := ParseName(oldloc)
	// walk both locations
	parent, err := ns.FWalk(ppath)
	if err != nil {
		return err
	}
	child, err := ns.FWalk(cpath)
	if err != nil {
		return err
	}
	
	//child.MayCreate = flags&p.MCREATE != 0 // FIXME: add MCREATE/MCACHE attrs to mounts.
	//child.MayCache = flags&p.MCACHE != 0
	
	return ns.Mnt.Mount(parent, child, flags)
}

/* Returns a list of things pointing here and things here points at (if a mount/union).
 */
func (ns *Namespace) LsMounts(path string) ([]string, []string, error) {
	parents := make([]string, 0)
	children := make([]string, 0)

	e := ParseName(path)
	fid, err := ns.FWalk(e)
	if err != nil {
		return parents, children, err
	}
	for _, p := range ns.Mnt.Mounted(fid.Type, fid.Dev, fid.Qid) {
		parents = append(parents, strings.Join(p.Cname,"/")) // TODO: this only gives the name up to its NSMOUNT
	}
	for _, c := range ns.Mnt.CheckMount(fid.Type, fid.Dev, fid.Qid) {
		children = append(children, strings.Join(c.Cname, "/")) // TODO: this only gives the name up to its NSMOUNT
	}
	return parents, children, nil
}

func (ns *Namespace) Umount(oldloc, newloc string) error {
	//var oper func()

	ppath := ParseName(newloc)
	cpath := ParseName(oldloc)
	// walk both locations
	parent, err := ns.FWalk(ppath)
	if err != nil {
		return err
	}
	child, err := ns.FWalk(cpath)
	if err != nil {
		return err
	}
	
	return ns.Mnt.Umount(parent, child)
}

