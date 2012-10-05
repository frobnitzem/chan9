package chan9

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
	Root *NSElem // This one must be of type NS_MOUNT, else there is no
			// server to accept 9p messages.

}


// List of path elements.
type Elemlist struct {
        elems []string
        mustbedir bool
}


/*
 * Create sub-slices of the names, breaking on '/'.
 * An empty string will give a nil nelem set.
 * A path ending in / or /. or /.//./ etc. will have
 * e.mustbedir = 1, so that we correctly
 * reject, e.g., "/adm/users/." when /adm/users is a file
 * rather than a directory.
 */
func Parsepath(name string) (e Elemlist) {
        e.elems = make([]string, 0)
        e.mustbedir = true // skip leading slash-dots
        n := 0
        for i, c := range name {
                if e.mustbedir {
                        if c != '/' {
                                if c != '.' || (len(name) > i+1 && name[i+1] != '/') {
                                        e.mustbedir = false
                                        n = i
                                }
                        }
                } else if c == '/' { // plan 7roll
                        /*if name[n:i] == ".." {
                                if l := len(e.elems); l > 0 {
                                        e.elems = e.elems[:l-1] }
                        } else {*/ // kill ".."
                        e.elems = append(e.elems, name[n:i])
                        e.mustbedir = true
                }
        }
        if i := len(name); !e.mustbedir && i > 0 {
                e.elems = append(e.elems, name[n:i])
        }
}

