/*  A simple forward<->backward mount mapping between fid-s.
 */

package chan9

import (
	"sync"
	"code.google.com/p/go9p/p"
	"fmt"
	"strings"
)

type FileID struct {
	Type    uint16
	Dev     uint32
	Qid	p.Qid
}

type mntstack struct {
	parent FileID
	child FileID
	next *mntstack
	prev *mntstack
}

/* Since the mnttab maintains Fid-s, they have to be GC-ed.
 * The definitive copy of ea. fid is stored with the Children[] structure,
 * inside a linked-list of Fids (prev/next).
 */

/* TODO: implement mutable mounts and intermediate folding
   TODO: fix Umount in mount.go to allow 'all parent' unmounting
 */
 
type Mnttab struct {
	sync.Mutex
	Parents  map[FileID][]*Fid
	Children map[FileID]*Fid
	FromDev  map[uint32]*mntstack
	ToDev    map[uint32]*mntstack
	Root     uint32
}

func (f FileID) String() string {
	s := ""
	if f.Type& NOREMAP != 0 {
		s = "*"
	}
	if f.Type&^NOREMAP != 0 {
		s += fmt.Sprintf("(%d)", f.Type)
	}
	s += fmt.Sprintf("%d:%d", f.Dev, f.Qid.Path)
	if f.Qid.Version != 0 {
		s += fmt.Sprintf(".%d", f.Qid.Version)
	}
	return s
}

/* Note: Type is not currently implemented, so it's not printed.
 */
func (m *Mnttab) PrintMnttab() {
	fmt.Printf("Mount Table:\n")
	m.Lock()
	defer m.Unlock()
	for p, c := range m.Children {
		if c == nil {
			continue
		}
		sp:="\t"
		fmt.Printf("  %s ->", p.String())
		for ; c != nil; c=c.next {
			fmt.Printf("%s%s = %s\n", sp, c.FileID.String(),
				   strings.Join(c.Cname,"/"))
			sp = "\t\t"
		}
	}

	fmt.Printf("  Parent Devices:")
	for d, l := range m.FromDev {
		if l == nil {
			continue
		}
		i := 0
		for ; l != nil; l=l.next {
			i += 1
		}
		fmt.Printf(" %d(%d)", d, i)
	}
	fmt.Printf("\n")
	fmt.Printf("  Child Devices:")
	for d, l := range m.ToDev {
		if l == nil {
			continue
		}
		i := 0
		for ; l != nil; l=l.next {
			i += 1
		}
		fmt.Printf(" %d(%d)", d, i)
	}
	fmt.Printf("\n")
}

func NewMnttab(dev uint32) (*Mnttab) {
	m := new(Mnttab)
	m.Root = dev
	m.Children = make(map[FileID]*Fid) // all 
	m.Parents  = make(map[FileID][]*Fid) // all mounts are union mounts.
	m.FromDev  = make(map[uint32]*mntstack)
	m.ToDev  = make(map[uint32]*mntstack)
	return m
}

func (m *Mnttab) Umount(child, parent *Fid) error {
	var s *mntstack

	m.Lock()
	defer m.Unlock()
	if parent == nil {
		parent = child
		child = nil
	}
	if parent == nil {
		return Ebaduse
	}

	pid := parent.FileID
	parent.Clunk()

	c, ok := m.Children[pid]
	if ! ok {
		return &p.Error{"mount not found", p.ENOENT}
	}
	if child == nil { // remove all mounts from parent, useful for remote fs
		for s = s.push(pid, c.FileID); c.next != nil; c = c.next {
		}
	} else {
		s = new(mntstack)
		s.parent = pid
		s.child = child.FileID
		child.Clunk()
	}
	m.rm_mnt(s)

	return nil
}

func (s *mntstack) push(parent, child FileID) *mntstack {
	sp := new(mntstack)
	/*sp.prev = s.prev
	if sp.prev != nil { // comment if always push/pop from top of stack.
		sp.prev.next = sp
	}*/
	sp.parent = parent
	sp.child = child
	sp.next = s
	if s != nil {
		s.prev = sp
	}
	return sp
}
// Traverse s to tack on sp.
func (s *mntstack) app(sp *mntstack) *mntstack {
	if s == nil {
		return sp
	}
	end := s
	for {
		next := end.next
		if next == nil {
			break
		}
		end = next
	}
	end.next = sp
	return s
}
/*
func (s *mntstack) pop() (FileID, *Fid, *mntstack) {
	if s == nil {
		return {}, nil, nil
	}
	if s.next != nil {
		s.next.prev = s.prev
	}
	return s.parent, s.child, s.next
}*/

/* Called with Mnttab's lock held.
 * Mnttab's lock has priority, and should be acquired
 * before Clntlist, if the latter is needed.
 */
func (m *Mnttab) rm_mnt(s *mntstack) {
	var is_noremap bool // mounts with noremap parents immediately precede self mounts

	for ; s != nil; s = s.next {
		is_noremap = s.parent.Type&NOREMAP != 0

		//if s.child.Type&NOREMAP == 0 { // NOREMAP-S should never get added to rm_mnt!
		dev := s.parent.Dev
		m.FromDev[dev] = m.FromDev[dev].remove_from(s.parent, s.child)
		dev = s.child.Dev
		m.ToDev[dev] = m.ToDev[dev].remove_from(s.parent, s.child)
		if dev != m.Root && m.ToDev[dev] == nil { // no more links to the child's device
			s = s.app(m.FromDev[dev])
		}
		//}
		
		m.Parents[s.child] = remove_from_sl(m.Parents[s.child], s.parent)
		clist := m.Children[s.parent]
		if is_noremap { // silently discard remapped parent, but don't Clunk.
			clist = remove_from_union(clist, s.parent, false)
		}
		m.Children[s.parent] = remove_from_union(clist, s.child, true)
	}
}

/*  Note that you cannot clunk a (parent or child) fid once it's sent to Mount.
 *  Also, the child should not be part of a union, since then the mount target
 *  is uncertain.
 */
func (m *Mnttab) Mount(child, parent *Fid, flags uint32) error {
	var err error

	/* Sanity checks. */
	if parent == nil {
		if child == nil {
			return Ebaduse
		}
		child.Clunk()
		return Ebaduse
	}
	if child == nil {
		parent.Clunk()
		return Ebaduse
	}
	if parent.Qid.Type&p.QTDIR == 0 && flags&p.MORDER != p.MREPL {
		err = &p.Error{"Cannot union mount a file, only a dir.", p.EINVAL}
		goto error
	}
	if (parent.Qid.Type&p.QTDIR) ^ (child.Qid.Type&p.QTDIR) != 0 {
		err = &p.Error{"Parent and child must both be either files or dirs.", p.EINVAL}
		goto error
	}
	if parent.prev != nil || parent.next != nil {
		err = &p.Error{"Cannot mount from a union, only a single fileid.", p.EINVAL}
		goto error
	}
	if child.prev != nil || child.next != nil {
		err = &p.Error{"Cannot mount a union, only a single fileid.", p.EINVAL}
		goto error
	}

	m.Lock()
	defer m.Unlock()

	// Require some ref. of parent's dev.
	if parent.Dev != m.Root && m.ToDev[parent.Dev] == nil  {
		err = &p.Error{"Cannot mount from a nonexistent device", p.ENOSYS}
		goto error
	}
	goto fine
error:
	child.Clunk()
	parent.Clunk()
	return err
fine:

	pid := parent.FileID
	cid := child.FileID
	ch  := m.Children[pid]
	// This requires a special kind of mount.
	if ch == nil && flags&p.MORDER != p.MREPL {
		parent.Type |= NOREMAP
	}

	// Update Device Listings - ensuring the present mount isn't
	// GC-ed during Mount.
	child.MayCreate = flags&p.MCREATE != 0
	child.MayCache = flags&p.MCACHE != 0

	m.FromDev[pid.Dev] = m.FromDev[pid.Dev].push(pid, cid)
	m.ToDev[  cid.Dev] = m.ToDev[  cid.Dev].push(pid, cid)

	// Update parent table
	if pl := m.Parents[cid]; pl == nil {
		m.Parents[cid] = make([]*Fid, 1)
		m.Parents[cid][0] = parent
	} else {
		m.Parents[cid] = append(pl, parent)
	}

	// Update child table
	if flags&p.MORDER == p.MREPL {
		var s *mntstack // unlink old.
		for ; ch != nil; ch=ch.next {
			s = s.push(pid, ch.FileID)
		}
		m.rm_mnt(s)

		if pid == cid { // self-replace.
			child.Type |= NOREMAP
		}
		m.Children[pid] = child
	} else {
		var lst *Fid // last child in existing union

		if ch == nil {
			parent.Type |= NOREMAP
			parent.MayCreate = true
			parent.MayCache = child.MayCache
			m.Children[pid] = parent
			ch = parent
			lst = parent
			// special case for parent GC, but this extra ref is
		} else { // redundant and simply ignored by rm_mnt
			var s *mntstack // for removing duplicate parent,child mnt
			for lst=ch; lst.next != nil; lst=lst.next {
				if lst.FileID == cid {
					s = s.push(pid, cid)
				}
			}
			m.rm_mnt(s)
		}

		switch flags&p.MORDER {
		case p.MAFTER:
			child.prev = lst
			lst.next = child
		case p.MBEFORE:
			child.next = ch
			m.Children[pid] = child
		}
	}
	return nil
}

/*  Check whether the given Fid is mounted,
    returning a slice of unions or nil.
    TODO: Make this loop and remove/combine intermediates (against Plan9 Spec)
 */
func (m *Mnttab) CheckMount(id FileID) *Fid {
	m.Lock()
	c := m.Children[id]
	m.Unlock()
	return c
}

/*  Check parents for those with a matching FileID
    to decide whether to step back through a mount.
 */
func (m *Mnttab) CheckParent(parent, child FileID) *Fid {
	mask_id := FileID{parent.Type|NOREMAP, parent.Dev, parent.Qid}
	ck_equiv := func(p FileID) bool {
		p.Type |= NOREMAP
		return p == mask_id
	}
	fmt.Printf("Checking Parents for %v against %v\n", child, parent)
	for _, p := range(m.Parents[child]) {
		fmt.Printf("  %v\n", p.FileID)
		if ck_equiv(p.FileID) {
			return p
		}
	}
	return nil
}

/*  List Parents of the current Fid (those mounting Fid).
 */
func (m *Mnttab) Mounted(id FileID) []*Fid {
	m.Lock()
	c := m.Parents[id]
	m.Unlock()
	return c
}

/*  Pull a FileID from a Fid
func (f *Fid) ID() FileID {
	var id FileID
	id.Type    = f.Type
	id.Dev     = f.Dev
	id.Qid     = f.Qid
	return id
}
 */

/* Unlink the children of the current NSElem,
   and remove loc->c (client connection) if not NSUNION.
func (ns *Namespace) unlink(loc *NSElem) {
	switch loc.Etype {
	case NSMOUNT:
		fallthrough
	case NSPASS:
		loc.c.edecref(&p.Error{"Sayonara", p.EINVAL})
	case NSUNION:
	}

	op := func() {
		loc.Child = make(map[string]*NSElem)
		loc.u = nil
		loc.Etype = NSPASS
	}
	ns.gc(op)
}
 */

/* Generic function to remove val from slice.
 */
func remove_from_sl(slice []*Fid, val FileID) []*Fid {
        var off int
        for i, v := range slice {
                slice[i-off] = slice[i]
                if off == 0 && v.FileID == val {
			v.Clunk()
                        off++
                }
        }
	if off == 0 {
		fmt.Printf("Error! tried to remove absentee fid from slice.\n")
	}
        return slice[:len(slice)-off]
}

/* Called with lock held.
 */
func (s *mntstack) remove_from(parent, child FileID) (*mntstack) {
	if s == nil {
		fmt.Printf("Error! tried to remove fid from non-existant mntstack.\n")
		return s
	}
	if s.parent == parent && s.child == child {
		next := s.next
		if next != nil {
			next.prev = s.prev
		}
		/* if s.prev != nil {
			s.prev.next = next
		} */
		return next
	}
	head := s
        for s=s.next; s != nil; s=s.next {
                if s.parent == parent && s.child == child {
			if s.prev != nil {
				s.prev.next = s.next
			}
			if s.next != nil {
				s.next.prev = s.prev
			}
			return head
		}
        }
	fmt.Printf("Error! tried to remove absentee fid from mntstack.\n")
        return head
}

// Remove the first elem. from the union.
func remove_from_union(s *Fid, v FileID, clunk bool) *Fid {
	if s == nil {
		fmt.Printf("Error! tried to remove fid from non-existant union.\n")
		return s
	}
	if s.FileID == v {
		next := s.next
		if next != nil {
			next.prev = s.prev
		}
		/* if s.prev != nil {
			s.prev.next = next
		} */
		if clunk {
			s.Clunk()
		}
		return next
	}
	head := s
        for s=s.next; s != nil; s=s.next {
                if s.FileID == v {
			if s.prev != nil {
				s.prev.next = s.next
			}
			if s.next != nil {
				s.next.prev = s.prev
			}
			if clunk {
				s.Clunk()
			}
			return head
		}
        }
	fmt.Printf("Error! tried to remove absentee fid from union.\n")
        return head
}

// Todo - check for cycles.
func (m *Mnttab) GC() {
}
