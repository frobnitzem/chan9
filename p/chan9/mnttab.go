/*  A simple forward<->backward mount mapping between fid-s.
 */

package chan9

import (
	"sync"
	"code.google.com/p/go9p/p"
	"fmt"
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

type Mnttab struct {
	sync.Mutex
	Parents  map[FileID][]FileID
	Children map[FileID]*Fid
	FromDev  map[uint32]*mntstack
	ToDev    map[uint32]*mntstack
	Root     uint32
}

func (m *Mnttab) PrintMnttab() {
	m.Lock()
	defer m.Unlock()
	fmt.Printf("Parents:\n  %v\nChildren:\n  %v\nFromDev:\n  %v\nToDev:\n  %v\n",
		m.Parents,m.Children,m.FromDev,m.ToDev)
}

func NewMnttab(dev uint32) (*Mnttab) {
	m := new(Mnttab)
	m.Root = dev
	m.Children = make(map[FileID]*Fid) // all 
	m.Parents  = make(map[FileID][]FileID) // all mounts are union mounts.
	m.FromDev  = make(map[uint32]*mntstack)
	m.ToDev  = make(map[uint32]*mntstack)
	m.FromDev[dev] = nil
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

	pid := parent.ID()
	parent.Clunk()

	c, ok := m.Children[pid]
	if ! ok {
		return &p.Error{"mount not found", p.ENOENT}
	}
	if child == nil { // remove all mounts from parent, useful for remote fs
		for s = s.push(pid, c.ID()); c.next != nil; c = c.next {
		}
	} else {
		s = new(mntstack)
		s.parent = pid
		s.child = child.ID()
		child.Clunk()
	}
	n := m.rm_mnt(s)
	if n == 0 {
		return &p.Error{"mount not found", p.ENOENT}
	}

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
func (m *Mnttab) rm_mnt(s *mntstack) int {
	var n int

	for ; s != nil; s = s.next {
		dev := s.parent.Dev
		m.FromDev[dev] = m.FromDev[dev].remove_from(s.parent, s.child)
		dev = s.child.Dev
		m.ToDev[dev] = m.ToDev[dev].remove_from(s.parent, s.child)
		if dev != m.Root && m.ToDev[dev] == nil { // no more links to the child's device
			s = s.app(m.FromDev[dev])
		}

		m.Parents[s.child] = remove_from_sl(m.Parents[s.child], s.parent)
		clist, np := remove_from_union(m.Children[s.parent], s.child)
		n += np
		if clist != nil && clist.next == nil {
			cp := clist.ID()
			cp.Type &= ^NOREMAP
			if cp == s.parent {
				clist = nil // remove self-shadows.
			}
		}
		m.Children[s.parent] = clist
	}
	return n
}

/*  Note that you cannot clunk a (parent or child) fid once it's sent to Mount.
 *  Also, the child should not be part of a union, since then the mount target
 *  is uncertain.
 */
func (m *Mnttab) Mount(child, parent *Fid, flags uint32) error {
	var err error

	/* Sanity checks. */
	if parent == nil || child == nil {
		return Ebaduse
	}
	if parent.Qid.Type&p.QTDIR == 0 && flags&p.MORDER != p.MREPL {
		child.Clunk()
		parent.Clunk()
		return &p.Error{"Cannot union mount a file, only a dir.", p.EINVAL}
	}
	if parent.prev != nil || parent.next != nil {
		child.Clunk()
		parent.Clunk()
		return &p.Error{"Cannot mount from a union, only a single fileid.", p.EINVAL}
	}
	if child.prev != nil || child.next != nil {
		child.Clunk()
		parent.Clunk()
		return &p.Error{"Cannot mount a union, only a single fileid.", p.EINVAL}
	}

	m.Lock()
	defer m.Unlock()

	pid := parent.ID()
	ch, ok := m.Children[pid]
	cid := child.ID()

	// Update Device Listings
	fdev, ok := m.FromDev[pid.Dev]
	tdev := m.ToDev[cid.Dev]
	if !ok {
		err = &p.Error{"Cannot mount from a nonexistent device", p.ENOSYS}
		goto err
	}
	child.prev = nil
	child.MayCreate = flags&p.MCREATE != 0
	child.MayCache = flags&p.MCACHE != 0

	m.FromDev[pid.Dev] = fdev.push(pid, cid)
	m.ToDev[child.Dev] = tdev.push(pid, cid)

	// Update parent table
	if pl := m.Parents[cid]; pl == nil {
		m.Parents[cid] = make([]FileID, 1)
		m.Parents[cid][0] = pid
	} else {
		m.Parents[cid] = append(pl, pid)
	}

	// Update child table
	if flags&p.MORDER == p.MREPL {
		if ok { // unlink old.
			var s *mntstack
			for ; ch != nil; ch=ch.next {
				s = s.push(pid, ch.ID())
			}
			m.rm_mnt(s)
		}
		m.Children[pid] = child
		parent.Clunk()
	} else {
		if ch == nil {
			parent.Type |= NOREMAP
			parent.MayCreate = true
			parent.MayCache = child.MayCache
			m.Children[pid] = parent
			ch = parent
		} else {
			parent.Clunk()
		}
		switch flags&p.MORDER {
		case p.MAFTER:
			for ; ch.next != nil; ch=ch.next {
			}
			child.prev = ch
			ch.next = child
		case p.MBEFORE:
			child.next = ch
			m.Children[pid] = child
		}
	}
	return nil
err:
	child.Clunk()
	parent.Clunk()
	return err
}

/*  Check whether the given Fid is mounted,
    returning a slice of unions or nil.
 */
func (m *Mnttab) CheckMount(Type uint16, dev uint32, qid p.Qid) *Fid {
	var id FileID
	id.Dev = dev
	id.Type = Type
	id.Qid = qid

	m.Lock()
	c := m.Children[id]
	m.Unlock()
	return c
}

/*  List Parents of the current Fid (those mounting Fid).
 */
func (m *Mnttab) Mounted(Type uint16, dev uint32, qid p.Qid) []FileID {
	var id FileID
	id.Type = Type
	id.Dev = dev
	id.Qid = qid

	m.Lock()
	c := m.Parents[id]
	m.Unlock()
	return c
}

/*  Pull a FileID from a Fid
 */
func (f *Fid) ID() FileID {
	var id FileID
	id.Type    = f.Type
	id.Dev     = f.Dev
	id.Qid     = f.Qid
	return id
}

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
func remove_from_sl(slice []FileID, val FileID) []FileID {
        var off int
        for i, v := range slice {
                slice[i-off] = slice[i]
                if v == val {
                        off++
                }
        }
        return slice[:len(slice)-off]
}

/* Called with lock held.
 */
func (s *mntstack) remove_from(parent, child FileID) (*mntstack) {
	var prev, ns *mntstack
	
	if s != nil {
		prev = s.prev
	}
	for s != nil  {
		if s.parent != parent || s.child != child {
			break
		}
		s = s.next
	}
	if s != nil {
		s.prev = prev
	}
	head := s
        for s != nil {
		ns = s.next
                if s.parent == parent && s.child == child {
			if s.prev != nil {
				s.prev.next = s.next
			}
			if s.next != nil {
				s.next.prev = s.prev
			}
		}
		s = ns
        }
        return head
}

func remove_from_union(s *Fid, v FileID) (*Fid, int) {
	var n int
	var prev *Fid
	var next *Fid
	
	if s != nil {
		prev = s.prev
	}
	for s != nil  {
		if s.ID() != v {
			break
		}
		n += 1
		next = s.next
		s.Clunk()
		s = next
	}
	if s != nil {
		s.prev = prev
	}
	head := s
        for s != nil {
		next = s.next
                if s.ID() == v {
			if s.prev != nil {
				s.prev.next = s.next
			}
			if s.next != nil {
				s.next.prev = s.prev
			}
			s.Clunk()
			n += 1
		}
		s = next
        }
        return head, n
}

// Todo - check for cycles.
func (m *Mnttab) GC() {
}
