// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The clnt package provides definitions and functions used to implement
// a 9P2000 file client.
package chan9

import (
	"code.google.com/p/go9p/p"
	"fmt"
	"log"
	"os"
	"net"
	"sync"
	"syscall"
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

type ClntList struct {
	sync.Mutex
	c map[uint32]*Clnt
	nextdev uint32
}

var DefaultDebuglevel int
var DefaultLogger *p.Logger
var clnts *ClntList

// The Clnt type represents a 9P2000 client. The client is connected to
// a 9P2000 file server and its methods can be used to access and manipulate
// the files exported by the server.
type Clnt struct {
	sync.Mutex
	Msize      uint32 // Maximum size of the 9P messages
	Dotu       bool   // If true, 9P2000.u protocol is spoken
	User       p.User
        Type uint16 // Although "ChanOps" interface deprecates
                        // the Type field, it could be informative.
        Dev uint32 // Device number for this channel
	Subpath    []string // "root" to begin requests from the channel
	//Root       *Fid   // Fid that points to subpath on the server - managed by mount and ns
	Debuglevel int    // Copied from ns
	Id         string // Info. about attached server,
			  // used when printing debug messages
	Log        *p.Logger

	conn     net.Conn
	tagpool  *pool // dedicated to this particular connection
	fidpool  *pool // points to pool of parent Namespace
	reqout   chan *Req
	done     chan bool
	reqfirst *Req
	reqlast  *Req
	err      error

	reqchan chan *Req
	tchan   chan *p.Fcall

	ref int // ref count.
}

type Req struct {
	sync.Mutex
	Clnt       *Clnt
	Tc         *p.Fcall
	Rc         *p.Fcall
	Err        error
	Done       chan *Req
	tag        uint16
	prev, next *Req
	fid        *Fid
}

func PrintClntList() {
	fmt.Printf("Connected Clients - Dev: name (refs)\n")
	clnts.Lock()
	defer clnts.Unlock()
	for d, c := range clnts.c {
		c.Lock()
		fmt.Printf("  %d: %s (%d)\n", d, c.Id, c.ref)
		c.Unlock()
	}
}

func (clnt *Clnt) Rpcnb(r *Req) error {
	var tag uint16

	if r.Tc.Type == p.Tversion {
		tag = p.NOTAG
	} else {
		tag = r.tag
	}

	p.SetTag(r.Tc, tag)
	clnt.Lock()
	if clnt.err != nil {
		clnt.Unlock()
		return clnt.err
	}

	if clnt.reqlast != nil {
		clnt.reqlast.next = r
	} else {
		clnt.reqfirst = r
	}

	r.prev = clnt.reqlast
	clnt.reqlast = r
	clnt.Unlock()

	clnt.reqout <- r
	return nil
}

func (clnt *Clnt) Rpc(tc *p.Fcall) (rc *p.Fcall, err error) {
	r := clnt.ReqAlloc()
	r.Tc = tc
	r.Done = make(chan *Req)
	err = clnt.Rpcnb(r)
	if err != nil {
		return
	}

	<-r.Done
	rc = r.Rc
	err = r.Err
	clnt.ReqFree(r)
	return
}

func (clnt *Clnt) Clunk(err error) {
	clnt.edecref(err)
}

func rm(clnt *Clnt, err error) {
	if clnt == nil {
		return
	}
	clnt.Lock()
	clnt.conn.Close()
	clnt.err = err
	clnt.done <- true

	/* send error to all pending requests */
	r := clnt.reqfirst
	clnt.reqfirst = nil
	clnt.reqlast = nil
	clnt.Unlock()
	for ; r != nil; r = r.next {
		r.Err = err
		if r.Done != nil {
			r.Done <- r
		}
	}

	clnts.Lock()
        delete(clnts.c, clnt.Dev)
	clnts.Unlock()

	if sop, ok := (interface{}(clnt)).(StatsOps); ok {
		sop.statsUnregister()
	}
}

func (clnt *Clnt) incref() (ref int) {
	clnt.Lock()
	clnt.ref++
	ref = clnt.ref
	clnt.Unlock()
	return
}

func (clnt *Clnt) decref() (ref int) {
	if clnt == nil {
		return 0
	}
	clnt.Lock()
	clnt.ref--
	ref = clnt.ref
	clnt.Unlock()
	return
}

// error message will be set if refcount goes to zero
// and client is sacked
func (clnt *Clnt) edecref(e error) (ref int) {
	ref = clnt.decref()
	if ref == 0 {
		rm(clnt, e)
	}

	return
}

func recv(clnt *Clnt) {
	buf := make([]byte, clnt.Msize*8)
	pos := 0
	for {
		if len(buf) < int(clnt.Msize) {
			b := make([]byte, clnt.Msize*8)
			copy(b, buf[0:pos])
			buf = b
			b = nil
		}

		n, oerr := clnt.conn.Read(buf[pos:len(buf)])
		if oerr != nil || n == 0 {
			rm(clnt,&p.Error{oerr.Error(), p.EIO})
			return
		}

		pos += n
		for pos > 4 {
			sz, _ := p.Gint32(buf)
			if pos < int(sz) {
				if len(buf) < int(sz) {
					b := make([]byte, clnt.Msize*8)
					copy(b, buf[0:pos])
					buf = b
					b = nil
				}

				break
			}

			fc, err, fcsize := p.Unpack(buf, clnt.Dotu)
			clnt.Lock()
			if err != nil {
				rm(clnt,err)
				return
			}

			if clnt.Debuglevel > 0 {
				clnt.logFcall(fc)
				if clnt.Debuglevel&DbgPrintPackets != 0 {
					log.Println("}-}", clnt.Id, fmt.Sprint(fc.Pkt))
				}

				if clnt.Debuglevel&DbgPrintFcalls != 0 {
					log.Println("}}}", clnt.Id, fc.String())
				}
			}

			var r *Req = nil
			for r = clnt.reqfirst; r != nil; r = r.next {
				if r.Tc.Tag == fc.Tag {
					break
				}
			}

			if r == nil {
				rm(clnt,&p.Error{"unexpected response", p.EINVAL})
				return
			}

			r.Rc = fc
			if r.prev != nil {
				r.prev.next = r.next
			} else {
				clnt.reqfirst = r.next
			}

			if r.next != nil {
				r.next.prev = r.prev
			} else {
				clnt.reqlast = r.prev
			}
			clnt.Unlock()

			if r.Tc.Type != r.Rc.Type-1 {
				if r.Rc.Type != p.Rerror {
					r.Err = &p.Error{"invalid response", p.EINVAL}
					log.Println(fmt.Sprintf("TTT %v", r.Tc))
					log.Println(fmt.Sprintf("RRR %v", r.Rc))
				} else {
					if r.Err == nil {
						r.Err = &p.Error{r.Rc.Error, syscall.Errno(r.Rc.Errornum)}
					}
				}
			}

			if r.Done != nil {
				r.Done <- r
			}

			pos -= fcsize
			buf = buf[fcsize:]
		}
	}
}

func send(clnt *Clnt) {
	for {
		select {
		case <-clnt.done:
			return

		case req := <-clnt.reqout:
			if clnt.Debuglevel > 0 {
				clnt.logFcall(req.Tc)
				if clnt.Debuglevel&DbgPrintPackets != 0 {
					log.Println("{-{", clnt.Id, fmt.Sprint(req.Tc.Pkt))
				}

				if clnt.Debuglevel&DbgPrintFcalls != 0 {
					log.Println("{{{", clnt.Id, req.Tc.String())
				}
			}

			for buf := req.Tc.Pkt; len(buf) > 0; {
				n, err := clnt.conn.Write(buf)
				if err != nil {
					/* just close the socket, will get signal on clnt.done */
					clnt.conn.Close()
					break
				}

				buf = buf[n:len(buf)]
			}
		}
	}
}


// Creates and initializes a new Clnt object. Doesn't send any data
// on the wire.
func NewClnt(c net.Conn, msize uint32, dotu bool) *Clnt {
	clnt := new(Clnt)
	clnt.conn = c
	clnt.Msize = msize
	clnt.Dotu = dotu
	clnt.User = p.OsUsers.Uid2User(os.Geteuid())
	clnt.Subpath = make([]string, 0) // root
	clnt.tagpool = newPool(uint32(p.NOTAG))
	clnt.fidpool = newPool(p.NOFID) // replace when mounting to NS!
	//clnt.fidpool = ns.fidpool
	clnt.reqout = make(chan *Req)
	clnt.done = make(chan bool)
	clnt.reqchan = make(chan *Req, 16)
	clnt.tchan = make(chan *p.Fcall, 16)
	clnt.ref = 1

	clnt.Debuglevel = DefaultDebuglevel
	clnt.Log = DefaultLogger

	clnt.Type = 0 //-- we have no special types for now
	clnts.Lock()
	clnt.Dev = clnts.nextdev
	clnts.c[clnt.Dev] = clnt
	clnts.nextdev++
	clnts.Unlock()

	go recv(clnt)
	go send(clnt)

	//if sop, ok := (interface{}(clnt)).(StatsOps); ok {
	//	sop.statsRegister()
	//}

	return clnt
}

// Establishes a new socket connection to the 9P server and creates
// a client object for it. Negotiates the dialect and msize for the
// connection. Returns a Clnt object, or Error.
func Connect(c net.Conn, msize uint32, dotu bool) (*Clnt, error) {
	clnt := NewClnt(c, msize, dotu)
	clnt.Id = c.RemoteAddr().String() + ":"
	ver := "9P2000"
	if clnt.Dotu {
		ver = "9P2000.u"
	}

	tc := p.NewFcall(clnt.Msize)
	err := p.PackTversion(tc, clnt.Msize, ver)
	if err != nil {
		return nil, err
	}

	rc, err := clnt.Rpc(tc)
	if err != nil {
		return nil, err
	}

	if rc.Msize < clnt.Msize {
		clnt.Msize = rc.Msize
	}

	clnt.Dotu = rc.Version == "9P2000.u" && clnt.Dotu
	return clnt, nil
}

// Creates a new Fid object for the client
func (clnt *Clnt) FidAlloc() *Fid {
	fid := new(Fid)
	fid.Fid = clnt.fidpool.getId()
	fid.Clnt = clnt
	fid.Dev = clnt.Dev
	fid.Type = clnt.Type
	fid.User = clnt.User
	fid.Cname = make([]string, 0)
	fid.Path = make([]FileID, 0)
	clnt.incref()

	return fid
}

func (clnt *Clnt) NewFcall() *p.Fcall {
	select {
	case tc := <-clnt.tchan:
		return tc
	default:
	}
	return p.NewFcall(clnt.Msize)
}

func (clnt *Clnt) FreeFcall(fc *p.Fcall) {
	if fc != nil && len(fc.Buf) >= int(clnt.Msize) {
		select {
		case clnt.tchan <- fc:
			break
		default:
		}
	}
}

func (clnt *Clnt) ReqAlloc() *Req {
	var req *Req
	select {
	case req = <-clnt.reqchan:
		break
	default:
		req = new(Req)
		req.Clnt = clnt
		req.tag = uint16(clnt.tagpool.getId())
	}
	return req
}

func (clnt *Clnt) ReqFree(req *Req) {
	clnt.FreeFcall(req.Tc)
	req.Tc = nil
	req.Rc = nil
	req.Err = nil
	req.Done = nil
	req.next = nil
	req.prev = nil

	select {
	case clnt.reqchan <- req:
		break
	default:
		clnt.tagpool.putId(uint32(req.tag))
	}
}

func (clnt *Clnt) logFcall(fc *p.Fcall) {
	if clnt.Debuglevel&DbgLogPackets != 0 {
		pkt := make([]byte, len(fc.Pkt))
		copy(pkt, fc.Pkt)
		clnt.Log.Log(pkt, clnt, DbgLogPackets)
	}

	if clnt.Debuglevel&DbgLogFcalls != 0 {
		f := new(p.Fcall)
		*f = *fc
		f.Pkt = nil
		clnt.Log.Log(f, clnt, DbgLogFcalls)
	}
}

func init() {
	clnts = new(ClntList)
	clnts.c = make(map[uint32]*Clnt)
	if sop, ok := (interface{}(clnts)).(StatsOps); ok {
		sop.statsRegister()
	}
}
