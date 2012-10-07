package main

// An interactive client for 9P servers.

import (
	"bufio"
	"code.google.com/p/go9p/p"
	"code.google.com/p/go9p/p/chan9"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

type Namespace chan9.Namespace

var addr = flag.String("addr", "127.0.0.1:5640", "network address")
var ouser = flag.String("user", "", "user to connect as")
var cmdfile = flag.String("file", "", "read commands from file")
var prompt = flag.String("prompt", "9p> ", "prompt for interactive client")
var debug = flag.Bool("d", false, "enable debugging (fcalls)")
var debugall = flag.Bool("D", false, "enable debugging (raw packets)")

type Cmd struct {
	fun  func(c *Namespace, s []string)
	help string
}

var cmds map[string]*Cmd
var cwd string = "/"

func init() {
	cmds = make(map[string]*Cmd)
	cmds["write"] = &Cmd{cmdwrite, "write file string [...]\t«write the unmodified string to file, create file if necessary»"}
	cmds["echo"] = &Cmd{cmdecho, "echo file string [...]\t«echo string to file (newline appended)»"}
	cmds["stat"] = &Cmd{cmdstat, "stat file [...]\t«stat file»"}
	cmds["ls"] = &Cmd{cmdls, "ls [-l] file [...]\t«list contents of directory or file»"}
	cmds["cd"] = &Cmd{cmdcd, "cd dir\t«change working directory»"}
	cmds["cat"] = &Cmd{cmdcat, "cat file [...]\t«print the contents of file»"}
	cmds["mkdir"] = &Cmd{cmdmkdir, "mkdir dir [...]\t«create dir on remote server»"}
	cmds["get"] = &Cmd{cmdget, "get file [local]\t«get file from remote server»"}
	cmds["put"] = &Cmd{cmdput, "put file [remote]\t«put file on the remote server as 'file'»"}
	cmds["pwd"] = &Cmd{cmdpwd, "pwd\t«print working directory»"}
	cmds["rm"] = &Cmd{cmdrm, "rm file [...]\t«remove file from remote server»"}
	cmds["help"] = &Cmd{cmdhelp, "help [cmd]\t«print available commands or help on cmd»"}
	cmds["quit"] = &Cmd{cmdquit, "quit\t«exit»"}
	cmds["exit"] = &Cmd{cmdquit, "exit\t«quit»"}
}

// local from cwd
func normpath(s string) string {
	if len(s) > 0 {
		if s[0] == '/' {
			return path.Clean(s)
		}
		return path.Clean(cwd + "/" + s)
	}
	return "/"
}

func b(mode uint32, s uint8) string {
	var bits = []string{"---", "--x", "-w-", "-wx", "r--", "r-x", "rw-", "rwx"}
	return bits[(mode>>s)&7]
}

// Convert file mode bits to string representation
func modetostr(mode uint32) string {
	d := "-"
	if mode&p.DMDIR != 0 {
		d = "d"
	} else if mode&p.DMAPPEND != 0 {
		d = "a"
	}
	return fmt.Sprintf("%s%s%s%s", d, b(mode, 6), b(mode, 3), b(mode, 0))
}

// Write the string s to remote file f. Create f if it doesn't exist
func writeone(c *Namespace, f, s string) {
	fname := normpath(f)
	file, oserr := c.FCreate(fname, 0666, p.OWRITE)
	if oserr != nil {
		file, oserr = c.FOpen(fname, p.OWRITE|p.OTRUNC)
		if oserr != nil {
			fmt.Fprintf(os.Stderr, "error opening %s: %v\n", fname, oserr)
			return
		}
	}
	defer file.Close()

	m, oserr := file.Write([]byte(s))
	if oserr != nil {
		fmt.Fprintf(os.Stderr, "error writing to %s: %v\n", fname, oserr)
		return
	}

	if m != len(s) {
		fmt.Fprintf(os.Stderr, "short write %s\n", fname)
		return
	}
}

// Write s[1:] (with appended spaces) to the file s[0]
func cmdwrite(c *Namespace, s []string) {
	fname := normpath(s[0])
	str := strings.Join(s[1:], " ")
	writeone(c, fname, str)
}

// Echo (append newline) s[1:] to s[0]
func cmdecho(c *Namespace, s []string) {
	fname := normpath(s[0])
	str := strings.Join(s[1:], " ") + "\n"
	writeone(c, fname, str)
}

// Stat the remote file f
func statone(c *Namespace, f string) {
	fname := normpath(f)

	stat, oserr := c.FStat(fname)
	if oserr != nil {
		fmt.Fprintf(os.Stderr, "error in stat %s: %v\n", fname, oserr)
		return
	}
	fmt.Fprintf(os.Stdout, "%s\n", stat)
}

func cmdstat(c *Namespace, s []string) {
	for _, f := range s {
		statone(c, normpath(f))
	}
}

func dirtostr(d *p.Dir) string {
	return fmt.Sprintf("%s %s %s %-8d\t\t%s", modetostr(d.Mode), d.Uid, d.Gid, d.Length, d.Name)
}

func lsone(c *Namespace, path string, long bool) {
	s := normpath(path)
	st, oserr := c.FStat(s)
	if oserr != nil {
		fmt.Fprintf(os.Stderr, "error stat: %v\n", oserr)
		return
	}
	if st.Mode&p.DMDIR != 0 {
		file, oserr := c.FOpen(s, p.OREAD)
		if oserr != nil {
			fmt.Fprintf(os.Stderr, "error opening dir: %s\n", oserr)
			return
		}
		defer file.Close()
		for {
			d, oserr := file.Readdir(0)
			if oserr != nil {
				fmt.Fprintf(os.Stderr, "error reading dir: %v\n", oserr)
			}
			if d == nil || len(d) == 0 {
				break
			}
			for _, dir := range d {
				if long {
					fmt.Fprintf(os.Stdout, "%s\n", dirtostr(dir))
				} else {
					os.Stdout.WriteString(dir.Name + "\n")
				}
			}
		}
	} else {
		fmt.Fprintf(os.Stdout, "%s\n", dirtostr(st))
	}
}

func cmdls(c *Namespace, s []string) {
	long := false
	if len(s) > 0 && s[0] == "-l" {
		long = true
		s = s[1:]
	}
	if len(s) == 0 {
		lsone(c, cwd, long)
	} else {
		for _, d := range s {
			lsone(c, d, long)
		}
	}
}

func walkone(c *Namespace, s string, fileok bool) {
	ncwd := normpath(s)

	fid, err := c.FWalk(ncwd)

	if err != nil { // FWalk clunk-s the newfid for us.
		fmt.Fprintf(os.Stderr, "walk error: %s\n", err)
		return
	}
	defer fid.Clunk()

	if fileok != true && (fid.Type&p.QTDIR == 0) {
		fmt.Fprintf(os.Stderr, "can't cd to file [%s]\n", ncwd)
		return
	}

	cwd = ncwd
}

func cmdcd(c *Namespace, s []string) {
	if s != nil {
		walkone(c, strings.Join(s, "/"), false)
	}
}

// Print the contents of f
func cmdcat(c *Namespace, s []string) {
	buf := make([]byte, 8192)
Outer:
	for _, f := range s {
		fname := normpath(f)
		file, oserr := c.FOpen(fname, p.OREAD)
		if oserr != nil {
			fmt.Fprintf(os.Stderr, "error opening %s: %v\n", f, oserr)
			continue Outer
		}
		defer file.Close()
		for {
			n, oserr := file.Read(buf)
			if oserr != nil && oserr != io.EOF {
				fmt.Fprintf(os.Stderr, "error reading %s: %v\n", f, oserr)
			}
			if n == 0 {
				break
			}
			os.Stdout.Write(buf[0:n])
		}
	}
}

// Create a single directory on remote server
func mkone(c *Namespace, s string) {
	fname := normpath(s)
	file, oserr := c.FCreate(fname, 0777|p.DMDIR, p.OREAD)
	if oserr != nil {
		fmt.Fprintf(os.Stderr, "error creating directory %s: %v\n", fname, oserr)
		return
	}
	file.Close()
}

// Create directories on remote server
func cmdmkdir(c *Namespace, s []string) {
	for _, f := range s {
		mkone(c, f)
	}
}

// Copy a remote file to local filesystem
func cmdget(c *Namespace, s []string) {
	var from, to string
	switch len(s) {
	case 1:
		from = normpath(s[0])
		_, to = path.Split(s[0])
	case 2:
		from, to = normpath(s[0]), s[1]
	default:
		fmt.Fprintf(os.Stderr, "from arguments; usage: get from to\n")
	}

	tofile, err := os.Create(to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %s for writing: %s\n", to, err)
		return
	}
	defer tofile.Close()

	file, ferr := c.FOpen(from, p.OREAD)
	if ferr != nil {
		fmt.Fprintf(os.Stderr, "error opening %s for writing: %s\n", to, err)
		return
	}
	defer file.Close()

	buf := make([]byte, 8192)
	for {
		n, oserr := file.Read(buf)
		if oserr != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %s\n", from, oserr)
			return
		}
		if n == 0 {
			break
		}

		m, err := tofile.Write(buf[0:n])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %s\n", to, err)
			return
		}

		if m != n {
			fmt.Fprintf(os.Stderr, "short write %s\n", to)
			return
		}
	}
}

// Copy a local file to remote server
func cmdput(c *Namespace, s []string) {
	var from, to string
	switch len(s) {
	case 1:
		_, to = path.Split(s[0])
		to = normpath(to)
		from = s[0]
	case 2:
		from, to = s[0], normpath(s[1])
	default:
		fmt.Fprintf(os.Stderr, "incorrect arguments; usage: put local [remote]\n")
	}

	fromfile, err := os.Open(from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %s for reading: %s\n", from, err)
		return
	}
	defer fromfile.Close()

	file, ferr := c.FOpen(to, p.OWRITE|p.OTRUNC)
	if ferr != nil {
		file, ferr = c.FCreate(to, 0666, p.OWRITE)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "error opening %s for writing: %s\n", to, err)
			return
		}
	}
	defer file.Close()

	buf := make([]byte, 8192)
	for {
		n, oserr := fromfile.Read(buf)
		if oserr != nil && oserr != io.EOF {
			fmt.Fprintf(os.Stderr, "error reading %s: %s\n", from, oserr)
			return
		}

		if n == 0 {
			break
		}

		m, oserr := file.Write(buf[0:n])
		if oserr != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", to, oserr)
			return
		}

		if m != n {
			fmt.Fprintf(os.Stderr, "short write %s\n", to)
			return
		}
	}
}

func cmdpwd(c *Namespace, s []string) { fmt.Fprintf(os.Stdout, cwd+"\n") }

// Remove f from remote server
func rmone(c *Namespace, f string) {
	fname := normpath(f)

	err := c.FRemove(fname)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error in stat %s", err)
		return
	}
}
// Remove one or more files from the server
func cmdrm(c *Namespace, s []string) {
	for _, f := range s {
		rmone(c, normpath(f))
	}
}

// Print available commands
func cmdhelp(c *Namespace, s []string) {
	cmdstr := ""
	if len(s) > 0 {
		for _, h := range s {
			v, ok := cmds[h]
			if ok {
				cmdstr = cmdstr + v.help + "\n"
			} else {
				cmdstr = cmdstr + "unknown command: " + h + "\n"
			}
		}
	} else {
		cmdstr = "available commands: "
		for k, _ := range cmds {
			cmdstr = cmdstr + " " + k
		}
		cmdstr = cmdstr + "\n"
	}
	fmt.Fprintf(os.Stdout, "%s", cmdstr)
}

func cmdquit(c *Namespace, s []string) { os.Exit(0) }

func cmd(c *Namespace, cmd string) {
	ncmd := strings.Fields(cmd)
	if len(ncmd) <= 0 {
		return
	}
	v, ok := cmds[ncmd[0]]
	if ok == false {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", ncmd[0])
		return
	}
	v.fun(c, ncmd[1:])
	return
}

func interactive(c *Namespace) {
	reader := bufio.NewReaderSize(os.Stdin, 8192)
	for {
		fmt.Print(*prompt)
		line, err := reader.ReadSlice('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "exiting...\n")
			break
		}
		str := strings.TrimSpace(string(line))
		// TODO: handle larger input lines by doubling buffer
		in := strings.Split(str, "\n")
		for i := range in {
			if len(in[i]) > 0 {
				cmd(c, in[i])
			}
		}
	}
}

func main() {
	var user p.User
	var err error
	var c *chan9.Clnt
	var file *chan9.File
	var ns *Namespace = chan9.NewNS()

	flag.Parse()

	naddr := *addr
	if *ouser != "" {
		ns.User = p.OsUsers.Uname2User(*ouser)
	}
	if *debug {
		ns.Debuglevel = 1
	}
	if *debugall {
		ns.Debuglevel = 2
	}

	c, err = ns.Dial(*addr)
	if err != nil {
		return
	}
	err = ns.Mount(c, nil, "/", chan9.MREPL, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error mounting %s: %s\n", naddr, err)
		os.Exit(1)
	}

	walkone(ns, "/", false)

	if file != nil {
		//process(c)
		fmt.Sprint(os.Stderr, "file reading unimplemented\n")
	} else if flag.NArg() > 0 {
		flags := flag.Args()
		for _, uc := range flags {
			cmd(ns, uc)
		}
	} else {
		interactive(ns)
	}

	return
}
