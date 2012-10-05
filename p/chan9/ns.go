package chan9

import (
	"code.google.com/p/go9p/p"
	"net"
	"sync"
)

/*
type NsOps interface {
        //ChanOps
        //Mount(from *ChanOps, to string)
        FMount(from, to string)
        Bind(from, to string)
        Clone() (*NsOps, error)
} */


// Etype-s
const (
	NS_PASS = iota // pass-through, no subterfuge
	NS_MOUNT	// start of mount-point for a channel
	NS_UNION	// union mount - provides a linked list to the mount-pts
)

type NSMount struct {
        Type uint16 // Although "ChanOps" interface deprecates
                        // the Type field, it could be informative.
        Dev uint32 // Device number for this channel
        Subpath string // "root" to begin requests from the channel.
        *Clnt // embed the channel interface (just the client for now)
		// contains "Root" = Fid of mounted root
}

type NSUnion struct {
	Spath []*NSElem // search path of files residing "here"
			// This implies their names are irrelevant,
			// over-written by name of NSElem that ref-s me.
}

/* The NSElem mirrors the directory tree, acting as an overlay to
 * connected 9p servers.
 * It is polymorphic according to Etype.
 */
type NSElem struct {
	Etype int
	Name string // name of dir. -- can construct a full path by rooting.
	MayCreate bool // change to a pointer inside NSUnion?
	Child map[string]*NSElem // dir tree - used when sub-dirs have mounts
	*NSMount // used if Etype == NS_MOUNT
	*NSUnion // used if Etype == NS_UNION
        Parent []*NSElem // list of parents
			 // This is important for GC-ing the namespace
			 // after mounts / binds have taken place.
                         // Indeterminism in naming the path is avoided in rooting
                         // by always rooting using the 1st in the list.
}

// The top-level namespace keeps track
// of the mounted p9 clients and the user's fid-s.
type Namespace struct {
	Tree *NSElem // This one must be of type NS_MOUNT, else there is no
			// server to accept 9p messages.
	clnts *ClntList
	sync.Mutex
	Debuglevel int    // =0 don't print anything, >0 print Fcalls, >1 print raw packets
	Root       *Fid   // Fid that points to the root directory
	Id         string // Used when printing debug messages
	Log        *p.Logger

	conn     net.Conn
	tagpool  *pool
	fidpool  *pool
	reqout   chan *Req
	done     chan bool
	reqfirst *Req
	reqlast  *Req
	err      error

	reqchan chan *Req
	tchan   chan *p.Fcall
}


// List of path elements.
type Elemlist struct {
        Elem []string
        Mustbedir bool
}

// A Fid type represents a file on the server. Fids are used for the
// low level methods that correspond directly to the 9P2000 message requests
type Fid struct {
	sync.Mutex
	Clnt   *Clnt // Client the fid belongs to
	Iounit uint32
	Type uint16   // Channel type (index of function call table)
	Dev uint32    // Server or device number distinguishing the server from others of the same type
	p.Qid         // The Qid description for the file
	Mode   uint8  // Open mode (one of p.O* values) (if file is open)
	Fid    uint32 // Fid number
	p.User        // The user the fid belongs to
	walked bool   // true if the fid points to a walked file on the server
}

// The file is similar to the Fid, but is used in the high-level client
// interface.
type File struct {
	fid    *Fid
	offset uint64
}

type pool struct {
	sync.Mutex
	need  int
	nchan chan uint32
	maxid uint32
	imap  []byte
}

type ClntList struct {
	sync.Mutex
	clntList, clntLast *Clnt
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
   If the result of this process is a null string, cleanname returns the string ".", representing the current directory. 
 */
func Parsename(name string) (e Elemlist) {
        e.Elem = make([]string, 0)
        e.Mustbedir = true // skip leading slash-dots
	rooted := name[0] == '/'
        n := 0

	addelem := func (s string) {
		if s == ".." {
			if l := len(e.Elem); l > 0 {
				if e.Elem[l-1] != ".." {
					e.Elem = e.Elem[:l-1]
					return
				}
			} else if rooted {
				return // skip if rooted
			}
		}
		e.Elem = append(e.Elem, s)
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
	if l := len(e.Elem); l == 0 {
		e.Elem = append(e.Elem, ".")
	}
	return
}

