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
	var file *chan9.File
	var d []*p.Dir
	var path []string

	flag.Parse()
	chan9.DefaultDebuglevel = *debuglevel

	if flag.NArg() == 0 {
		path = make([]string, 1)
		path[0] = "/"
	} else {
		path = flag.Args()
	}

	c, err := chan9.Dial(*addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening connection to %s: %s\n", *addr, err)
		return
	}
	ns, err := chan9.NSFromClnt(c, nil, chan9.MREPL, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error mounting %s: %s\n", *addr, err)
		os.Exit(1)
	}

	for _, arg := range path {
		path := chan9.ParseName(arg)
		file, err = ns.FOpen(path, p.OREAD)
		if err != nil {
			os.Stderr.WriteString("Error: " + err.Error() + "\n")
			continue
		}
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
		} else if ! path.Mustbedir {
			os.Stdout.WriteString(path.String() + "\n")
		} else {
			os.Stderr.WriteString("Error: file not found.\n")
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
