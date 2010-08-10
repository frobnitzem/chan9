// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The srv package provides definitions and functions used to implement
// a 9P2000 file server.
package srv

import (
	"net"
	"sync"
	"syscall"
	"go9p.googlecode.com/hg/p"
)

type reqStatus int

const (
	reqFlush     reqStatus = (1 << iota) /* request is flushed (no response will be sent) */
	reqWork                              /* goroutine is currently working on it */
	reqResponded                         /* response is already produced */
	reqSaved                             /* no response was produced after the request is worked on */
)

var Eunknownfid *p.Error = &p.Error{"unknown fid", syscall.EINVAL}
var Enoauth *p.Error = &p.Error{"no authentication required", syscall.EINVAL}
var Einuse *p.Error = &p.Error{"fid already in use", syscall.EINVAL}
var Ebaduse *p.Error = &p.Error{"bad use of fid", syscall.EINVAL}
var Eopen *p.Error = &p.Error{"fid already opened", syscall.EINVAL}
var Enotdir *p.Error = &p.Error{"not a directory", syscall.ENOTDIR}
var Eperm *p.Error = &p.Error{"permission denied", syscall.EPERM}
var Etoolarge *p.Error = &p.Error{"i/o count too large", syscall.EINVAL}
var Ebadoffset *p.Error = &p.Error{"bad offset in directory read", syscall.EINVAL}
var Edirchange *p.Error = &p.Error{"cannot convert between files and directories", syscall.EINVAL}
var Enouser *p.Error = &p.Error{"unknown user", syscall.EINVAL}
var Enotimpl *p.Error = &p.Error{"not implemented", syscall.EINVAL}

// Authentication operations. The file server should implement them if
// it requires user authentication. The authentication in 9P2000 is
// done by creating special authentication fids and performing I/O
// operations on them. Once the authentication is done, the authentication
// fid can be used by the user to get access to the actual files.
type AuthOps interface {
	// AuthInit is called when the user starts the authentication
	// process on Fid afid. The user that is being authenticated
	// is referred by afid.User. The function should return the Qid
	// for the authentication file, or an Error if the user can't be
	// authenticated
	AuthInit(afid *Fid, aname string) (*p.Qid, *p.Error)

	// AuthDestroy is called when an authentication fid is destroyed.
	AuthDestroy(afid *Fid)

	// AuthCheck is called after the authentication process is finished
	// when the user tries to attach to the file server. If the function
	// returns nil, the authentication was successful and the user has
	// permission to access the files.
	AuthCheck(fid *Fid, afid *Fid, aname string) *p.Error

	// AuthRead is called when the user attempts to read data from an
	// authentication fid.
	AuthRead(afid *Fid, offset uint64, data []byte) (count int, err *p.Error)

	// AuthWrite is called when the user attempts to write data to an
	// authentication fid.
	AuthWrite(afid *Fid, offset uint64, data []byte) (count int, err *p.Error)
}

// Connection operations. These should be implemented if the file server
// needs to be called when a connection is opened or closed.
type ConnOps interface {
	ConnOpened(*Conn)
	ConnClosed(*Conn)
}

// Fid operations. This interface should be implemented if the file server
// needs to be called when a Fid is destroyed.
type FidOps interface {
	FidDestroy(*Fid)
}

// Request operations. This interface should be implemented if the file server
// needs to bypass the default request process, or needs to perform certain
// operations before the (any) request is processed, or before (any) response
// sent back to the client.
type ReqProcessOps interface {
	// Called when a new request is received from the client. If the
	// interface is not implemented, (req *Req) srv.Process() method is
	// called. If the interface is implemented, it is the user's
	// responsibility to call srv.Process. If srv.Process isn't called,
	// Fid, Afid and Newfid fields in Req are not set, and the ReqOps
	// methods are not called.
	ReqProcess(*Req)

	// Called when a request is responded, i.e. when (req *Req)srv.Respond()
	// is called and before the response is sent. If the interface is not
	// implemented, (req *Req) srv.PostProcess() method is called to finalize
	// the request. If the interface is implemented and ReqProcess calls
	// the srv.Process method, ReqRespond should call the srv.PostProcess
	// method.
	ReqRespond(*Req)
}

// Flush operation. This interface should be implemented if the file server
// can flush pending requests. If the interface is not implemented, requests
// that were passed to the file server implementation won't be flushed.
// The flush method should call the (req *Req) srv.Flush() method if the flush
// was successful so the request can be marked appropriately.
type FlushOp interface {
	Flush(*Req)
}

// Request operations. This interface should be implemented by all file servers.
// The operations correspond directly to most of the 9P2000 message types.
type ReqOps interface {
	Attach(*Req)
	Walk(*Req)
	Open(*Req)
	Create(*Req)
	Read(*Req)
	Write(*Req)
	Clunk(*Req)
	Remove(*Req)
	Stat(*Req)
	Wstat(*Req)
}

// The Srv type contains the basic fields used to control the 9P2000
// file server. Each file server implementation should create a value
// of Srv type, initialize the values it cares about and pass the
// struct to the (Srv *) srv.Start(ops) method together with the object
// that implements the file server operations.
type Srv struct {
	sync.Mutex
	Msize       uint32    // Maximum size of the 9P2000 messages supported by the server
	Dotu        bool      // If true, the server supports the 9P2000.u extension
	Debuglevel  int       // 0==don't print anything, >1 print 9P messages, >2 print raw data
	Upool       p.Users   // Interface for finding users and groups known to the file server
	Maxpend     int       // Maximum pending outgoing requests
	Ngoroutines int       // Number of goroutines handling requests, if 0, create a gorotine for each request
	Reqin       chan *Req // Incoming requests

	ops interface{} // operations

	connlist *Conn // List of connections
}

// The Conn type represents a connection from a client to the file server
type Conn struct {
	sync.Mutex
	Srv   *Srv
	Msize uint32 // maximum size of 9P2000 messages for the connection
	Dotu  bool   // if true, both the client and the server speak 9P2000.u
	Id    string // used when printing debug messages

	conn     net.Conn
	fidpool  map[uint32]*Fid
	reqfirst *Req
	reqlast  *Req

	reqout     chan *Req
	rchan      chan *p.Fcall
	done       chan bool
	prev, next *Conn
}

// The Fid type identifies a file on the file server.
// A new Fid is created when the user attaches to the file server (the Attach
// operation), or when Walk-ing to a file. The Fid values are created
// automatically by the srv implementation. The FidDestroy operation is called
// when a Fid is destroyed.
type Fid struct {
	sync.Mutex
	fid       uint32
	refcount  int
	opened    bool        // True if the Fid is opened
	Fconn     *Conn       // Connection the Fid belongs to
	Omode     uint8       // Open mode (p.O* flags), if the fid is opened
	Type      uint8       // Fid type (p.QT* flags)
	Diroffset uint64      // If directory, the next valid read position
	User      p.User      // The Fid's user
	Aux       interface{} // Can be used by the file server implementation for per-Fid data
}

// The Req type represents a 9P2000 request. Each request has a
// T-message (Tc) and a R-message (Rc). If the ReqProcessOps don't
// override the default behavior, the implementation initializes Fid,
// Afid and Newfid values and automatically keeps track on when the Fids
// should be destroyed.
type Req struct {
	sync.Mutex
	Tc     *p.Fcall // Incoming 9P2000 message
	Rc     *p.Fcall // Outgoing 9P2000 response
	Fid    *Fid     // The Fid value for all messages that contain fid[4]
	Afid   *Fid     // The Fid value for the messages that contain afid[4] (Tauth and Tattach)
	Newfid *Fid     // The Fid value for the messages that contain newfid[4] (Twalk)
	Conn   *Conn    // Connection that the request belongs to

	status     reqStatus
	flushreq   *Req
	prev, next *Req
}

// The Start method should be called once the file server implementor
// initializes the Srv struct with the preferred values. It sets default
// values to the fields that are not initialized and creates the goroutines
// required for the server's operation. The method receives an empty
// interface value, ops, that should implement the interfaces the file server is
// interested in. Ops must implement the ReqOps interface.
func (srv *Srv) Start(ops interface{}) bool {
	if _, ok := (ops).(ReqOps); !ok {
		return false
	}

	srv.ops = ops
	if srv.Upool == nil {
		srv.Upool = p.OsUsers
	}

	if srv.Msize < p.IOHDRSZ {
		srv.Msize = p.MSIZE
	}

	srv.Reqin = make(chan *Req, srv.Maxpend)
	n := srv.Ngoroutines
	for i := 0; i < n; i++ {
		go srv.work()
	}

	return true
}

func (req *Req) process() {
	req.Lock()
	flushed := (req.status & reqFlush) != 0
	if !flushed {
		req.status |= reqWork
	}
	req.Unlock()

	if flushed {
		req.Respond()
	}

	if rop, ok := (req.Conn.Srv.ops).(ReqProcessOps); ok {
		rop.ReqProcess(req)
	} else {
		req.Process()
	}

	req.Lock()
	req.status &= ^reqWork
	if !(req.status&reqResponded != 0) {
		req.status |= reqSaved
	}
	req.Unlock()
}

func (srv *Srv) work() {
	for req := <-srv.Reqin; req != nil; req = <-srv.Reqin {
		req.process()
	}
}

// Performs the default processing of a request. Initializes
// the Fid, Afid and Newfid fields and calls the appropriate
// ReqOps operation for the message. The file server implementer
// should call it only if the file server implements the ReqProcessOps
// within the ReqProcess operation.
func (req *Req) Process() {
	conn := req.Conn
	srv := conn.Srv
	tc := req.Tc

	if tc.Fid != p.NOFID && tc.Type != p.Tattach {
		srv.Lock()
		req.Fid = conn.FidGet(tc.Fid)
		srv.Unlock()
		if req.Fid == nil {
			req.RespondError(Eunknownfid)
			return
		}
	}

	switch req.Tc.Type {
	default:
	unknown:
		req.RespondError(&p.Error{"unknown message type", syscall.ENOSYS})

	case p.Tversion:
		srv.version(req)

	case p.Tauth:
		srv.auth(req)

	case p.Tattach:
		srv.attach(req)

	case p.Tflush:
		srv.flush(req)

	case p.Twalk:
		srv.walk(req)

	case p.Topen:
		srv.open(req)

	case p.Tcreate:
		srv.create(req)

	case p.Tread:
		srv.read(req)

	case p.Twrite:
		srv.write(req)

	case p.Tclunk:
		srv.clunk(req)

	case p.Tremove:
		srv.remove(req)

	case p.Tstat:
		srv.stat(req)

	case p.Twstat:
		srv.wstat(req)
	}
}

// Performs the post processing required if the (*Req) Process() method
// is called for a request. The file server implementer should call it
// only if the file server implements the ReqProcessOps within the
// ReqRespond operation.
func (req *Req) PostProcess() {
	srv := req.Conn.Srv

	/* call the post-handlers (if needed) */
	switch req.Tc.Type {
	case p.Tauth:
		srv.authPost(req)

	case p.Tattach:
		srv.attachPost(req)

	case p.Twalk:
		srv.walkPost(req)

	case p.Topen:
		srv.openPost(req)

	case p.Tcreate:
		srv.createPost(req)

	case p.Tread:
		srv.readPost(req)

	case p.Tclunk:
		srv.clunkPost(req)

	case p.Tremove:
		srv.removePost(req)
	}

	if req.Fid != nil {
		req.Fid.DecRef()
		req.Fid = nil
	}

	if req.Afid != nil {
		req.Afid.DecRef()
		req.Afid = nil
	}

	if req.Newfid != nil {
		req.Newfid.DecRef()
		req.Newfid = nil
	}
}

// The Respond method sends response back to the client. The req.Rc value
// should be initialized and contain valid 9P2000 message. In most cases
// the file server implementer shouldn't call this method directly. Instead
// one of the RespondR* methods should be used.
func (req *Req) Respond() {
	conn := req.Conn
	req.Lock()
	status := req.status
	req.status |= reqResponded
	req.status &= ^reqWork
	req.Unlock()

	if (status & reqResponded) != 0 {
		return
	}

	/* remove the request and all requests flushing it */
	conn.Lock()
	if req.prev != nil {
		req.prev.next = req.next
	} else {
		conn.reqfirst = req.next
	}

	if req.next != nil {
		req.next.prev = req.prev
	} else {
		conn.reqlast = req.prev
	}

	for freq := req.flushreq; freq != nil; freq = freq.flushreq {
		if freq.prev != nil {
			freq.prev.next = freq.next
		} else {
			conn.reqfirst = freq.next
		}

		if freq.next != nil {
			freq.next.prev = freq.prev
		} else {
			conn.reqlast = freq.prev
		}
	}
	conn.Unlock()

	if rop, ok := (req.Conn.Srv.ops).(ReqProcessOps); ok {
		rop.ReqRespond(req)
	} else {
		req.PostProcess()
	}

	if (status & reqFlush) == 0 {
		conn.reqout <- req
	}

	for freq := req.flushreq; freq != nil; freq = freq.flushreq {
		if (freq.status & reqFlush) == 0 {
			conn.reqout <- freq
		}
	}
}

// Should be called to cancel a request. Should only be callled
// from the Flush operation if the FlushOp is implemented.
func (req *Req) Flush() {
	req.Lock()
	req.status |= reqFlush
	req.Unlock()
}

// Lookup a Fid struct based on the 32-bit identifier sent over the wire.
// Returns nil if the fid is not found. Increases the reference count of
// the returned fid. The user is responsible to call DecRef once it no
// longer needs it.
func (conn *Conn) FidGet(fidno uint32) *Fid {
	conn.Lock()
	fid, present := conn.fidpool[fidno]
	conn.Unlock()
	if present {
		fid.IncRef()
	}

	return fid
}

// Creates a new Fid struct for the fidno integer. Returns nil
// if the Fid for that number already exists. The returned fid
// has reference count set to 1.
func (conn *Conn) FidNew(fidno uint32) *Fid {
	conn.Lock()
	_, present := conn.fidpool[fidno]
	if present {
		conn.Unlock()
		return nil
	}

	fid := new(Fid)
	fid.fid = fidno
	fid.refcount = 1
	fid.Fconn = conn
	conn.fidpool[fidno] = fid
	conn.Unlock()

	return fid
}

// Increase the reference count for the fid.
func (fid *Fid) IncRef() {
	fid.Lock()
	fid.refcount++
	fid.Unlock()
}

// Decrease the reference count for the fid. When the
// reference count reaches 0, the fid is no longer valid.
func (fid *Fid) DecRef() {
	fid.Lock()
	fid.refcount--
	n := fid.refcount
	fid.Unlock()

	if n > 0 {
		return
	}

	conn := fid.Fconn
	conn.Lock()
	conn.fidpool[fid.fid] = nil, false
	conn.Unlock()

	if fop, ok := (conn.Srv.ops).(FidOps); ok {
		fop.FidDestroy(fid)
	}
}