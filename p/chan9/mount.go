package chan9

import (
	"code.google.com/p/go9p/p"
	"strings"
)


// The real mount command.
// -- see http://plan9.bell-labs.com/sys/doc/lexnames.html (but note Cnames are
//                      implemented as Qid lists, since names would)
//    and http://man.cat-v.org/inferno/2/sys-bind
//    The call to Attach creates a fid and runs Clnt.incref(), which will be destroyed
//    if the ns.Root is ever clunk()-ed
//    so call Clnt.incref() if you need to keep it.
func (ns *Namespace) Mount(clnt *Clnt, afd *Fid, oldloc string, flags uint32, aname string) error {
	var e Elemlist
	var parent *Fid

	if flags > p.MMASK-1 {
		return &p.Error{"bad mount flags", p.EINVAL}
	}
	fid, err := clnt.Attach(afd, clnt.User, aname)
	if err != nil {
		return err
	}
	// walk the client to reach clnt.Subpath
	if len(clnt.Subpath) > 0 {
		var Qids []p.Qid
		Qids, err = fid.Walk(fid, clnt.Subpath)
		if err != nil {
			goto err
		}
		if len(Qids) != len(clnt.Subpath) {
			err = &p.Error{"subpath not found on client", p.ENOENT}
			goto err
		}
	}

	e = ParseName(oldloc)
	parent, err = ns.FWalk(e) // walk to fid on parent side

	if err != nil {
		goto err
	}
	if cap(fid.Cname) < 1+len(clnt.Subpath) {
		fid.Cname = make([]string, 1+len(clnt.Subpath))
	} else {
		fid.Cname = fid.Cname[:1+len(clnt.Subpath)]
	}
	fid.Cname[0] = clnt.Id+":"
	copy(fid.Cname[1:], clnt.Subpath)
	err = ns.Mnt.Mount(fid, parent, flags)
	if err != nil {
		parent.Clunk()
		goto err
	}
	clnt.fidpool = ns.fidpool

	return nil
err:
	fid.Clunk()
	return err
}

// Mount's cousin (to, from), since we re-direct "from (=newloc)" to "to (=oldloc)".
// Note that the arguments are in the SAME order compared to ln -s and mount.
// parent -> child is read as 'the parent references the child'
// even though the child (e.g. /dev/...) is usually thought of as pre-existing,
// the parent -> child idea is more fitting for the filesystem hierarchy.
func (ns *Namespace) Bind(cname, pname string, flags uint32) error {
	if flags > p.MMASK-1 {
		return &p.Error{"bad bind flags", p.EINVAL}
	}
	ppath := ParseName(pname)
	cpath := ParseName(cname)
	// walk both locations
	parent, err := ns.FWalk(ppath)
	if err != nil {
		return err
	}
	child, err := ns.FWalk(cpath)
	if err != nil {
		return err
	}
	
	return ns.Mnt.Mount(child, parent, flags)
}

/* Returns a list of things pointing here and things here points at (if a mount/union).
 * TODO: This routine gives crazy answers because walking to the given string
 * causes substitution of parents -> children, so parents are always returned...
 * We need a walk-before + partial walk mechanism.
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
		parents = append(parents, strings.Join(p.Cname,"/"))
	}
	for c := ns.Mnt.CheckMount(fid.Type, fid.Dev, fid.Qid); c != nil; c=c.next {
		children = append(children, strings.Join(c.Cname, "/"))
	}
	return parents, children, nil
}

/* TODO: This routine never finds a mount because it's already substituted parents -> children.
 */
func (ns *Namespace) Umount(cname, pname string) error {
	//var oper func()
	var child *Fid

	ppath := ParseName(pname)
	cpath := ParseName(cname)
	// walk both locations
	parent, err := ns.FWalk(ppath)
	if err != nil {
		return err
	}

	//if cpath != "" { // leave as nil to unmount all from parent
	child, err = ns.FWalk(cpath)
	if err != nil {
		return err
	}
	//}
	
	return ns.Mnt.Umount(child, parent)
}

