package main

import (
	"code.google.com/p/go9p/p"
	"code.google.com/p/go9p/p/chan9"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
)

var debuglevel = flag.Int("d", 0, "debuglevel")
var addr = flag.String("addr", "127.0.0.1:5640", "server address")

func main() {
	var err error
	var c *chan9.Clnt
	var file *chan9.File
	var d []*p.Dir
	var path []string
	var ns *chan9.Namespace = chan9.NewNS()

	flag.Parse()
	chan9.DefaultDebuglevel = *debuglevel

	if flag.NArg() == 0 {
		path = make([]string, 1)
		path[0] = "/"
	} else {
		path = flag.Args()
	}

	c, err = ns.Dial(*addr)
	if err != nil {
		return
	}
	err = ns.Mount(c, nil, "/", chan9.MREPL, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error mounting %s: %s\n", *addr, err)
		os.Exit(1)
	}

	for _, arg := range path {
		file, err = ns.FOpen(arg, p.OREAD)
		if file.Fid.Qid.Type & p.QTDIR > 0 {
			if err != nil {
				log.Println(err)
				continue
			}
			for {
				d, err = file.Readdir(0)
				if d == nil || len(d) == 0 || err != nil {
					break
				}

				for _, di := range(d) {
					os.Stdout.WriteString(di.Name + "\n")
				}
			}
		} else {
			os.Stdout.WriteString(arg + "\n")
			//os.Stdout.WriteString(file.Fid.Cname + "\n")
		}
		file.Close()

		if err != nil && err != io.EOF {
			log.Println(err)
			err = nil
			continue
		}
	}
	ns.Close()

	return

}
