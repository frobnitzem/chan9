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
