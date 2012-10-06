package chan9

import (
	"code.google.com/p/go9p/p"
	"sync"
)

// Debug flags
const (
	DbgPrintFcalls  = (1 << iota) // print all 9P messages on stderr
	DbgPrintPackets               // print the raw packets on stderr
	DbgLogFcalls                  // keep the last N 9P messages (can be accessed over http)
	DbgLogPackets                 // keep the last N 9P messages (can be accessed over http)
)

type StatsOps interface {
	statsRegister()
	statsUnregister()
}

var DefaultDebuglevel int
var DefaultLogger *p.Logger

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
	NS_PASS  = 0	// pass-through, no subterfuge
	NS_MOUNT = 1	// start of mount-point for a channel
	NS_UNION = 2	// union mount - provides a linked list to the mount-pts
)

/*type NSMount struct {
        Type uint16 // Although "ChanOps" interface deprecates
                        // the Type field, it could be informative.
        Dev uint32 // Device number for this channel
        *Clnt // embed the channel interface (just the client for now)
		// contains "Root" = Fid of mounted root
}*/

type NSUnion struct {
	Spath []*NSElem // Search path of files residing "here"
			// This implies their names are irrelevant,
			// over-written by name of NSElem that ref-s me.
}

/* The NSElem mirrors the directory tree, acting as an overlay to
 * connected 9p servers.
 * It is polymorphic according to Etype.
 */
type NSElem struct {
	Etype int
	Cname []string // path taken to create elem
	MayCreate bool // have to store original create/mount info.
	*Clnt // used if Etype == NS_MOUNT
	*NSUnion // used if Etype == NS_UNION
	Child map[string]*NSElem // dir tree - used if ETYPE != NS_UNION
        Parent []*NSElem // list of parents
			 // This is important for GC-ing the namespace
			 // after mounts / binds have taken place.
                         // Indeterminism in naming the path is avoided
                         // by checking the Cname with the issuing call's '..'.
}

type ClntList struct {
	sync.Mutex
	c map[uint32]*Clnt
	nextdev uint32
}

// The top-level namespace keeps track
// of the mounted p9 clients and the user's fid-s.
type Namespace struct {
	Tree *NSElem // This one must be of type NS_MOUNT, else there is no
			// server to accept 9p messages.
	sync.Mutex
	Debuglevel int    // =0 don't print anything, >0 print Fcalls, >1 print raw packets
	Root       *Fid   // Fid that points to the root directory
	Id         string // Used when printing debug messages
	Log        *p.Logger

	tagpool  *pool
	fidpool  *pool
	err      error

	clnts *ClntList
}

func (ns *Namespace) init() {
        ns.clnts = new(ClntList)
        ns.tagpool = newPool(uint32(p.NOTAG))
        ns.fidpool = newPool(p.NOFID)
        if sop, ok := (interface{}(ns.clnts)).(StatsOps); ok {
                sop.statsRegister()
	}
	ns.Debuglevel = DefaultDebuglevel
	ns.Log = DefaultLogger
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
	Cname	[]string
	Iounit uint32
	Type uint16   // Channel type (index of function call table) -- FYI
	Dev uint32    // Server or device number distinguishing the server from others of the same type
			// duplicates Clnt * info
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

func (ns *Namespace) logFcall(fc *p.Fcall) {
	if ns.Debuglevel&DbgLogPackets != 0 {
		pkt := make([]byte, len(fc.Pkt))
		copy(pkt, fc.Pkt)
		ns.Log.Log(pkt, ns, DbgLogPackets)
	}

	if ns.Debuglevel&DbgLogFcalls != 0 {
		f := new(p.Fcall)
		*f = *fc
		f.Pkt = nil
		ns.Log.Log(f, ns, DbgLogFcalls)
	}
}

