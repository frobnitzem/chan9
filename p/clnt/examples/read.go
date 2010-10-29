package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"go9p.googlecode.com/hg/p"
	"go9p.googlecode.com/hg/p/clnt"
)

var debuglevel = flag.Int("d", 0, "debuglevel")
var addr = flag.String("addr", "127.0.0.1:5640", "network address")

func main() {
	var n int
	var user p.User
	var err *p.Error
	var c *clnt.Clnt
	var file *clnt.File

	flag.Parse()
	user = p.OsUsers.Uid2User(os.Geteuid())
	clnt.DefaultDebuglevel = *debuglevel
	c, err = clnt.Mount("tcp", *addr, "", user)
	if err != nil {
		goto error
	}

	if flag.NArg() != 1 {
		log.Println("invalid arguments")
		return
	}

	file, err = c.FOpen(flag.Arg(0), p.OREAD)
	if err != nil {
		goto error
	}

	buf := make([]byte, 8192)
	for {
		n, err = file.Read(buf)
		if err != nil {
			goto error
		}

		if n == 0 {
			break
		}

		os.Stdout.Write(buf[0:n])
	}

	file.Close()
	return

error:
	log.Println(fmt.Sprintf("Error: %s %d", err.Error, err.Errornum))
}
