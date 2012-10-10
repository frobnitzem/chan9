package chan9

import (
	"strings"
	"unicode/utf8"
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

/*type NSMount struct {
        Type uint16 // Although "ChanOps" interface deprecates
                        // the Type field, it could be informative.
        Dev uint32 // Device number for this channel
        *Clnt // embed the channel interface (just the client for now)
		// contains "Root" = Fid of mounted root
}*/

/*
type NSUnion struct {
	Spath []*NSElem // Search path of files residing "here"
			// This implies their names are irrelevant,
			// over-written by name of NSElem that ref-s me.
}*/

/* The NSElem mirrors the directory tree, acting as an overlay to
 * connected 9p servers.
 * It is polymorphic according to Etype.
 */
type NSElem struct {
	Etype int
	Cname []string // path taken to create elem
	MayCreate bool // have to store original create/mount info.
	c *Clnt // used if Etype == NSMOUNT
	u []*NSElem // used if Etype == NSUNION
	Child map[string]*NSElem // dir tree - used if ETYPE != NSUNION
        Parent []*NSElem // list of parents
			 // This is important for GC-ing the namespace
			 // after mounts / binds have taken place.
                         // Indeterminism in naming the path is avoided
                         // by checking the Cname with the issuing call's '..'.
}

/* NSElem */
func (mhead *NSElem) Birth(child string, m *NSElem) (*NSElem) {
	if m == nil {
		m := new(NSElem)
		m.Etype = NSPASS
		m.Cname = make([]string, len(mhead.Cname)+1)
		copy(m.Cname, mhead.Cname)
		m.Cname[len(mhead.Cname)] = child
		m.c = mhead.c
		m.MayCreate = mhead.MayCreate
		m.Parent = make([]*NSElem, 1)
		m.Parent[0] = mhead
		m.Child = make(map[string]*NSElem)
	}

	switch mhead.Etype {
	case NSUNION:
		for _,d := range mhead.u {
			if d.MayCreate {
				d.Birth(child, m)
				break
			}
		}
	case NSPASS:
		fallthrough
	case NSMOUNT:
		fallthrough
	default:
		mhead.Child[child] = m
	}

	return m
}

func (mhead *NSElem) Lookup(child string) (next *NSElem) {
	switch mhead.Etype {
	case NSUNION:
		for _,d := range mhead.u {
			next := d.Lookup(child)
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
