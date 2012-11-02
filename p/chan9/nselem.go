package chan9

import (
	"strings"
	"unicode/utf8"
	"code.google.com/p/go9p/p"
)

// List of path elements.
type Elemlist struct {
        Elems []string
        Ref rune
        Mustbedir bool
}

/*
type NsOps interface {
        //ChanOps
        //Mount(from *ChanOps, to string)
        FMount(from, to string)
        Bind(from, to string)
        Clone() (*NsOps, error)
} */


// Mutually exclusive Etype-s
const (
	NSPASS  = 0	// pass-through, no subterfuge
	NSMOUNT = 1	// start of mount-point for a channel
	NSUNION = 2	// union mount - provides a linked list to the mount-pts
)

/* The NSElem mirrors the directory tree, acting as an overlay to
 * connected 9p servers.
 * It is polymorphic according to Etype.
 */
type NSElem struct {
	Etype int
	Cname []string // path taken to create elem
	MayCreate bool // have to store original create/mount info.
	MayCache bool // may cache some data.
	c *Clnt // used if Etype == NSMOUNT or NSPASS
	u []*NSElem // used if Etype == NSUNION
	Child map[string]*NSElem // dir tree - used if ETYPE != NSUNION
        Parent []*NSElem // list of parents
}

/* We're staying with the same client, and just adding NSPASS types.
 * The parent should not be NSUNION.
 */
func (ns *Namespace) Birth(mhead *NSElem, child string) (*NSElem) {
	m := new(NSElem)
	m.Etype = NSPASS
	m.Cname = make([]string, len(mhead.Cname)+1)
	copy(m.Cname, mhead.Cname)
	m.Cname[len(mhead.Cname)] = child
	m.c = mhead.c
	m.c.incref()
	m.MayCreate = mhead.MayCreate
	m.Parent = make([]*NSElem, 1)
	m.Parent[0] = mhead
	m.Child = make(map[string]*NSElem)

	switch mhead.Etype {
	case NSUNION:
		return nil
	case NSPASS:
		fallthrough
	case NSMOUNT:
		fallthrough
	default:
		ns.replace_child(mhead, child, m)
	}

	return m
}

func (ns *Namespace) replace_child(p *NSElem, name string, c *NSElem) {
	oc := p.Child[name]
	if oc != nil {
		ns.unlink(oc)
	}
	p.Child[name] = c
}

/*
func (ns *Namespace) append_child(p *NSElem, name string, c *NSElem) {
	oc := p.Child[name]
	if oc != nil {
		ns.unlink(c)
	} else {
		p.Child[name] = c
	}
}*/

func (mhead *NSElem) Lookup(child string) (next *NSElem) {
	switch mhead.Etype {
	case NSUNION:
		for _,d := range mhead.u {
			next = d.Lookup(child)
			if next != nil {
				break
			}
		}
	case NSPASS:
		fallthrough
	case NSMOUNT:
		fallthrough
	default:
		next = mhead.Child[child]
	}
	return
}

func PathJoin(from, add []string) []string {
	var ndotdot int

	for ndotdot=0; ndotdot < len(add); ndotdot++ {
		if add[ndotdot] != ".." {
			break
		}
	}
	nkeep := len(from)-ndotdot
	if nkeep < 0 {
		nkeep = 0
	}

	l := nkeep+len(add)-ndotdot
	out := make([]string, l)
	copy(out[:nkeep], from)
	copy(out[nkeep:], add[ndotdot:])
	return out
}

func (ns *Namespace) RootPath(path string) Elemlist {
	e := ParseName(path)
        switch e.Ref {
        case '/':
                return e
        case '.':
		e.Elems = PathJoin(ns.Cwd, e.Elems)
		e.Ref = '/'
                return e
	} //default: // TODO -- implement # names
        return e
}

// Split a path (containing no ..-s) into mount head + path inside mount components.
func mhead_split(root *NSElem, elem[]string) (*NSElem, []string) {
	head := root
	mhead := head

	mi := 0
	for i,p := range elem {
		next := head.Lookup(p)
		if next == nil {
			break
		}
		if next.Etype == NSMOUNT {
			mhead = next
			mi = i+1
		}
		head = next
	}

	return mhead, elem[mi:]
}

/*
 * Create sub-slices of the names, breaking on '/'.
 * An empty string will give a nil nelem set.
 * A path ending in / or /. or /.//./ etc. will have
 * e.Mustbedir = 1, so that we correctly
 * reject, e.g., "/adm/users/." when /adm/users is a file
 * rather than a directory.
 */
/* Cleanname is analogous to the URL-cleaning rules defined in RFC 1808
   [Field95], although the rules are slightly different. Cleanname iteratively
   does the following until no further processing can be done: 
   1. Reduce multiple slashes to a single slash.
   2. Eliminate . path name elements (the current directory).
   3. Eliminate .. path name elements (the parent directory) and the non-. non-.., element that precedes them.
   4. Eliminate .. elements that begin a rooted path, that is, replace /.. by / at the beginning of a path.
   5. Leave intact .. elements that begin a non-rooted path.
   If the result of this process is a null string, cleanname returns an empty list.
 */
func ParseName(name string) (e Elemlist) {
        e.Elems = make([]string, 0)
        e.Mustbedir = true // skip leading slash-dots
	c,l := utf8.DecodeRuneInString(name)
	switch c {
	case '/':
		e.Ref = '/'
		name = name[l:]
	case '#':
		e.Ref = '#'
		name = name[l:]
	default:
		e.Ref = '.'
	}
        n := 0

	addelem := func (s string) {
		if s == ".." {
			if l := len(e.Elems); l > 0 {
				if e.Elems[l-1] != ".." {
					e.Elems = e.Elems[:l-1]
					return
				}
			} else if e.Ref == '/' {
				return // skip if rooted
			}
		}
		e.Elems = append(e.Elems, s)
	}
        for i, c := range name {
                if e.Mustbedir {
                        if c != '/' {
                                if c != '.' || (len(name) > i+1 && name[i+1] != '/') {
                                        e.Mustbedir = false
                                        n = i
                                }
                        }
                } else if c == '/' {
                        e.Mustbedir = true
			addelem(name[n:i])
                }
        }
        if i := len(name); !e.Mustbedir && i > 0 {
		if name[n:i] == ".." {
			e.Mustbedir = true }
		addelem(name[n:i])
        }
	/*if l := len(e.Elems); l == 0 {
		e.Elems = append(e.Elems, ".")
	}*/
	return
}

func (e *Elemlist) String() string {
	var hd string = ""
	var tl string = ""
	switch e.Ref {
	case '/':
		hd = "/"
	case '#':
		hd = "#"
	}
	if e.Mustbedir {
		tl = "/"
	}

	return hd + strings.Join(e.Elems, "/") + tl
}

/* Unlink the children of the current NSElem,
   and remove loc->c (client connection) if not NSUNION.
 */
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

func (ns *Namespace) remove(loc *NSElem) {
	/*for _, par := range loc.Parent {
		switch par.Etype {
		case NSPASS:
			fallthrough
		case NSMOUNT:
			delete(par.Child, loc)
		case NSUNION:
			par.u = remove_from(par.u, loc)
	} - done by GC */
	ns.unlink(loc)
}

/* Container for GC-ed alterations to the namespace.
   It lists the currently reachable objects, calls
   its callback, then re-lists the reachable objects,
   removing any NSElems that have fallen off the wagon.
 */
func (ns *Namespace) gc(op func()) {
	elems := make([]*NSElem, 0)
	f := func(e *NSElem) bool {
		//elems = append(elems, e)
		return true
	}
	elems = visit(f, ns.Root, elems)
	i := 0
	f = func(e *NSElem) bool {
		j, ok := find_elem(elems[i:], e)
		if !ok {
			return true
		}
		j += i
		t := elems[j]
		elems[j] = elems[i]
		elems[i] = t
		i++
		return true
	}
	op()
	visit(f, ns.Root, make([]*NSElem, len(elems)))

	for _, par := range elems[i:] {
		if par.Etype != NSUNION {
			par.c.edecref(&p.Error{"GC Sayonara", p.EINVAL})
		}
	}
	for _, child := range elems[:i] {
		for _, par := range elems[i:] {
			child.Parent = remove_from(child.Parent, par)
		}
	}
}

func visit(f func(*NSElem)bool, loc *NSElem, prev []*NSElem) []*NSElem {
	_, ok := find_elem(prev, loc)
	if ok {
		return prev
	}
	prev = append(prev, loc)
	if !f(loc) {
		return prev
	}
	switch loc.Etype {
	case NSMOUNT:
		fallthrough
	case NSPASS:
		for _, child := range loc.Child {
			prev = visit(f, child, prev)
		}
	case NSUNION:
		for _, child := range loc.u {
			prev = visit(f, child, prev)
		}
	}
	return prev
}

func find_elem(elems []*NSElem, e *NSElem) (int, bool) {
	var i int
	var v *NSElem

	for i, v = range elems {
		if v == e {
			return i, true
		}
	}
	return len(elems), false
}

/* Generic function to remove val from slice.
 */
func remove_from(slice []*NSElem, val *NSElem) []*NSElem {
        var off int
        for i, v := range slice {
                slice[i-off] = slice[i]
                if v == val {
                        off++
                }
        }
        return slice[:len(slice)-off]
}

