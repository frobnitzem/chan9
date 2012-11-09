/*  A simple forward<->backward mount mapping between fid-s.
 */

package chan9

import (
	"sync"
	"code.google.com/p/go9p/p"
)

type FileID struct {
	Type    uint16
	Dev     uint32
	Qid	p.Qid
}

type mntstack struct {
	parent *Fid
	child *Fid
	next *mntstack
	prev *mntstack
}

type Mnttab struct {
	sync.Mutex
	Children map[FileID][]*Fid
	Parents  map[FileID][]*Fid
	FromDev  map[uint32]*mntstack
	ToDev    map[uint32]*mntstack
}

func NewMnttab(dev uint32) (*Mnttab) {
	m := new(Mnttab)
	m.Children = make(map[FileID][]*Fid) // all 
	m.Parents  = make(map[FileID][]*Fid) // all mounts are union mounts.
	m.FromDev  = make(map[uint32]*mntstack)
	m.ToDev  = make(map[uint32]*mntstack)
	m.FromDev[dev] = nil
	return m
}

func (m *Mnttab) Umount(parent, child *Fid) error {
	m.Lock()
	defer m.Unlock()

	_, ok := m.Children[parent.ID()]
	if ! ok {
		return &p.Error{"mount not found", p.ENOENT}
	}
	s := new(mntstack)
	s.parent = parent
	s.child = child
	m.rm_mnt(s)

	return nil
}

func (s *mntstack) push(parent, child *Fid) *mntstack {
	sp := new(mntstack)
	sp.prev = s.prev
	/*if sp.prev != nil { // comment if always push/pop from top of stack.
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
func (s *mntstack) pop() (*Fid, *Fid, *mntstack) {
	if s == nil {
		return nil, nil, nil
	}
	if s.next != nil {
		s.next.prev = s.prev
	}
	return s.parent, s.child, s.next
}*/

func (m *Mnttab) AddDev(dev uint32) {
	m.Lock()
	defer m.Unlock()
	m.FromDev[dev] = nil
	/*d := m.ToDev[dev]
	rm_mnt(d)
	d := m.ToDev[dev]
	rm_mnt(d)
	m.Devices[dev] = nil
	*/
}

/* Called with Mnttab's lock held.
 * Mnttab's lock has priority, and should be acquired
 * before Clntlist, if the latter is needed.
 */
func (m *Mnttab) rm_mnt(s *mntstack) bool {
	var n int
	for s != nil {
		dev := s.child.Dev
		m.ToDev[dev], n = m.ToDev[dev].remove_from(s.parent, s.child)
		if m.ToDev == nil { // no more links to the child's device
			s = s.app(m.FromDev[dev])
		}
		if n == 0 {
			return false
		}
		pid := s.parent.ID()
		cid := s.child.ID()
		m.Parents[pid] = remove_from_sl(m.Parents[pid], s.child)
		m.Children[cid] = remove_from_sl(m.Children[cid], s.parent)
		s = s.next
	}
	return true
}

func (m *Mnttab) Mount(parent, child *Fid, flags uint32) error {
	m.Lock()
	defer m.Unlock()
	
	pid := parent.ID()
	ch, ok := m.Children[pid]
	cid := child.ID()

	// Update Device Listings
	fdev, ok := m.FromDev[parent.Dev]
	tdev := m.ToDev[child.Dev]
	if !ok {
		return &p.Error{"Cannot mount from a nonexistent device", p.ENOSYS}
	}

	// Update parent table
	m.FromDev[parent.Dev] = fdev.push(parent, child)
	m.ToDev[child.Dev] = tdev.push(parent, child)

	if m.Parents[cid] == nil {
		m.Parents[cid] = make([]*Fid, 1)
		m.Parents[cid][0] = parent
	} else {
		m.Parents[cid] = append(m.Parents[cid], parent)
	}

	// Update child table
	if m.Children[pid] == nil {
		m.Children[pid] = make([]*Fid, 1)
		m.Children[pid][0] = child
		return nil
	}
	if flags&p.MORDER == p.MREPL {
		if ok { // unlink old.
			var ls *mntstack
			for _, c := range(ch) {
				s := new(mntstack)
				s.parent = parent
				s.child = c
				s.next = ls
				ls = s
			}
			m.rm_mnt(ls)
		}
		m.Children[pid] = append(m.Children[pid], child)
	} else {
		switch flags&p.MORDER {
		case p.MAFTER:
			m.Children[pid] = append(m.Children[pid], child)
		case p.MBEFORE:
			l := len(m.Children[pid])
			if cap(m.Children[pid]) < l+1 {
				m.Children[pid] = make([]*Fid, l+3)[:l+1]
			} else {
				m.Children[pid] = m.Children[pid][:l+1]
			}
			copy(m.Children[pid][1:l+1], m.Children[pid][:l])
			m.Children[pid][0] = child
		}
	}
	return nil
}

/*  Check whether the given Fid is mounted,
    returning a slice of unions or nil.
 */
func (m *Mnttab) CheckMount(Type uint16, dev uint32, qid p.Qid) []*Fid {
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
func (m *Mnttab) Mounted(Type uint16, dev uint32, qid p.Qid) []*Fid {
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
func remove_from_sl(slice []*Fid, val *Fid) []*Fid {
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
func (s *mntstack) remove_from(parent, child *Fid) (*mntstack, int) {
	var n int
	var prev *mntstack
	
	if s != nil {
		prev = s.prev
	}
	for s != nil  {
		if s.parent != parent || s.child != child {
			break
		}
		s.parent.Clunk()
		s.child.Clunk()
		n += 1
		s = s.next
	}
	if s != nil {
		s.prev = prev
	}
	head := s
        for head != nil {
                if head.parent == parent && head.child == child {
			head.parent.Clunk()
			head.child.Clunk()
			n += 1
			head.prev.next = head.next
			head.next.prev = head.prev
		}
		head = head.next
        }
        return s, n
}
