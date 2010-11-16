package clnt

import (
	"fmt"
	"io"
	"http"
	"go9p.googlecode.com/hg/p"
)


func (clnt *Clnt) ServeHTTP(c http.ResponseWriter, r *http.Request) {
	io.WriteString(c, fmt.Sprintf("<html><body><h1>Client %s</h1>", clnt.Id))
	defer io.WriteString(c, "</body></html>")

	// fcalls
	if clnt.Debuglevel&DbgLogFcalls != 0 {
		fs := clnt.Log.Filter(clnt, DbgLogFcalls)
		io.WriteString(c, fmt.Sprintf("<h2>Last %d 9P messages</h2>", len(fs)))
		for _, l := range fs {
			fc := l.Data.(*p.Fcall)
			if fc.Type != 0 {
				io.WriteString(c, fmt.Sprintf("<br>%s", fc))
			}
		}
	}
}

func clntServeHTTP(c http.ResponseWriter, r *http.Request) {
	io.WriteString(c, fmt.Sprintf("<html><body>"))
	defer io.WriteString(c, "</body></html>")

	clntLock.Lock()
	if clntList == nil {
		io.WriteString(c, "no clients")
	}

	for clnt := clntList; clnt != nil; clnt = clnt.next {
		io.WriteString(c, fmt.Sprintf("<a href='/go9p/clnt/%s'>%s</a><br>", clnt.Id, clnt.Id))
	}
	clntLock.Unlock()
}

func (clnt *Clnt) statsRegister() {
	http.Handle("/go9p/clnt/"+clnt.Id, clnt)
}

func (clnt *Clnt) statsUnregister() {
	http.Handle("/go9p/clnt/"+clnt.Id, nil)
}


func statsRegister() {
	http.HandleFunc("/go9p/clnt", clntServeHTTP)
}
