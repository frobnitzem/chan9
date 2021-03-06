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

var addr = flag.String("addr", "127.0.0.1:5640", "network address")
var ouser = flag.String("user", "", "user to connect as")
var cmdfile = flag.String("file", "", "read commands from file")
var prompt = flag.String("prompt", "9p> ", "prompt for interactive client")
var debug = flag.Bool("d", false, "enable debugging (fcalls)")
var debugall = flag.Bool("D", false, "enable debugging (raw packets)")

var ns *chan9.Namespace

type Cmd struct {
	fun  func(s []string)
	help string
}

var cmds map[string]*Cmd

func init() {
	cmds = make(map[string]*Cmd)
	cmds["write"]   = &Cmd{cmdwrite, "write file string [...]\t«write the unmodified string to file, create file if necessary»"}
	cmds["echo"]    = &Cmd{cmdecho, "echo file string [...]\t«echo string to file (newline appended)»"}
	cmds["stat"]    = &Cmd{cmdstat, "stat file [...]\t«stat file»"}
	cmds["ls"]      = &Cmd{cmdls, "ls [-l] file [...]\t«list contents of directory or file»"}
	cmds["cd"]      = &Cmd{cmdcd, "cd dir\t«change working directory»"}
	cmds["cat"]     = &Cmd{cmdcat, "cat file [...]\t«print the contents of file»"}
	cmds["mkdir"]   = &Cmd{cmdmkdir, "mkdir dir [...]\t«create dir on remote server»"}
	cmds["get"]     = &Cmd{cmdget, "get file [local]\t«get file from remote server»"}
	cmds["put"]     = &Cmd{cmdput, "put file [remote]\t«put file on the remote server as 'file'»"}
	cmds["mount"]   = &Cmd{cmdmount, "mount [-bacCq] remote mountpoint\t«mount the remote server on mountpoint»"}
	cmds["bind"]    = &Cmd{cmdbind, "bind [-bacq] target mountpoint\t«mount the target directory on mountpoint»"}
	cmds["netstat"] = &Cmd{cmdnetstat, "netstat\t«list open connections and reference numbers»"}
	cmds["lsmount"] = &Cmd{cmdlsmount, "lsmount mountpoint\t«list the mounts from/to mountpoint»"}
	cmds["umount"]  = &Cmd{cmdumount, "umount remote mountpoint\t«remove the given mount»"}
	cmds["pwd"]     = &Cmd{cmdpwd, "pwd\t«print working directory»"}
	cmds["rm"]      = &Cmd{cmdrm, "rm file [...]\t«remove file from remote server»"}
	cmds["help"]    = &Cmd{cmdhelp, "help [cmd]\t«print available commands or help on cmd»"}
	cmds["quit"]    = &Cmd{cmdquit, "quit\t«exit»"}
	cmds["exit"]    = &Cmd{cmdquit, "exit\t«quit»"}
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
func writeone(fname chan9.Elemlist, s string) {
	file, oserr := ns.FCreate(fname, 0666, p.OWRITE)
	if oserr != nil {
		fmt.Printf("error creating %s: %v\n", fname.String(), oserr)
		fmt.Printf("trying open...\n")
		file, oserr = ns.FOpen(fname, p.OWRITE|p.OTRUNC)
		if oserr != nil {
			fmt.Fprintf(os.Stderr, "error opening %s: %v\n", fname.String(), oserr)
			return
		}
	}
	defer file.Close()

	m, oserr := file.Write([]byte(s))
	if oserr != nil {
		fmt.Fprintf(os.Stderr, "error writing to %s: %v\n", fname.String(), oserr)
		return
	}

	if m != len(s) {
		fmt.Fprintf(os.Stderr, "short write %s\n", fname.String())
		return
	}
}

// Write s[1:] (with appended spaces) to the file s[0]
func cmdwrite(s []string) {
	fname := chan9.ParseName(s[0])
	str := strings.Join(s[1:], " ")
	writeone(fname, str)
}

// Echo (append newline) s[1:] to s[0]
func cmdecho(s []string) {
	fname := chan9.ParseName(s[0])
	str := strings.Join(s[1:], " ") + "\n"
	writeone(fname, str)
}

// Stat the remote file f
func statone(fname chan9.Elemlist) {
	stat, oserr := ns.FStat(fname)
	if oserr != nil {
		fmt.Fprintf(os.Stderr, "error in stat %s: %v\n", fname.String(), oserr)
		return
	}
	fmt.Fprintf(os.Stdout, "%#v\n", stat)
}

func cmdstat(s []string) {
	for _, f := range s {
		statone(chan9.ParseName(f))
	}
}

func dirtostr(d *p.Dir) string {
	return fmt.Sprintf("%s %s %s %-8d\t\t%s", modetostr(d.Mode), d.Uid, d.Gid, d.Length, d.Name)
}

func lsone(s chan9.Elemlist, long bool) {
	st, oserr := ns.FStat(s)
	if oserr != nil {
		fmt.Fprintf(os.Stderr, "error stat: %v\n", oserr)
		return
	}
	if st.Mode&p.DMDIR != 0 {
		file, oserr := ns.FOpen(s, p.OREAD)
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

func cmdls(s []string) {
	long := false
	if len(s) > 0 && s[0] == "-l" {
		long = true
		s = s[1:]
	}
	if len(s) == 0 {
		var cwd = chan9.Elemlist {Ref:'.', Elems:make([]string, 0)}
		lsone(cwd, long)
	} else {
		for _, d := range s {
			lsone(chan9.ParseName(d), long)
		}
	}
}

func cmdcd(s []string) {
	if s == nil || len(s) < 1 {
		return
	}
	err := ns.Cd(s[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "cd error: %s\n", err)
	}
}

// Print the contents of f
func cmdcat(s []string) {
	buf := make([]byte, 8192)
Outer:
	for _, f := range s {
		fname := chan9.ParseName(f)
		file, oserr := ns.FOpen(fname, p.OREAD)
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
func mkone(fname chan9.Elemlist) {
	file, oserr := ns.FCreate(fname, 0777|p.DMDIR, p.OREAD)
	if oserr != nil {
		fmt.Fprintf(os.Stderr, "error creating directory %s: %v\n", fname.String(), oserr)
		return
	}
	file.Close()
}

// Create directories on remote server
func cmdmkdir(s []string) {
	for _, f := range s {
		mkone(chan9.ParseName(f))
	}
}

// Copy a remote file to local filesystem
func cmdget(s []string) {
	var from chan9.Elemlist
	var to string
	switch len(s) {
	case 1:
		from = chan9.ParseName(s[0])
		_, to = path.Split(s[0])
	case 2:
		from, to = chan9.ParseName(s[0]), s[1]
	default:
		fmt.Fprintf(os.Stderr, "from arguments; usage: get from to\n")
	}

	tofile, err := os.Create(to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %s for writing: %s\n", to, err)
		return
	}
	defer tofile.Close()

	file, ferr := ns.FOpen(from, p.OREAD)
	if ferr != nil {
		fmt.Fprintf(os.Stderr, "error opening %s for reading: %s\n", from.String(), err)
		return
	}
	defer file.Close()

	buf := make([]byte, 8192)
	for {
		n, oserr := file.Read(buf)
		if oserr != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %s\n", from.String(), oserr)
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
func cmdput(s []string) {
	var from string
	var to chan9.Elemlist
	switch len(s) {
	case 1:
		_, to_s := path.Split(s[0])
		to = chan9.ParseName(to_s)
		from = s[0]
	case 2:
		from, to = s[0], chan9.ParseName(s[1])
	default:
		fmt.Fprintf(os.Stderr, "incorrect arguments; usage: put local [remote]\n")
	}

	fromfile, err := os.Open(from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening %s for reading: %s\n", from, err)
		return
	}
	defer fromfile.Close()

	file, ferr := ns.FOpen(to, p.OWRITE|p.OTRUNC)
	if ferr != nil {
		file, ferr = ns.FCreate(to, 0666, p.OWRITE)
		if ferr != nil {
			fmt.Fprintf(os.Stderr, "error opening %s for writing: %s\n", to.String(), err)
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
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", to.String(), oserr)
			return
		}

		if m != n {
			fmt.Fprintf(os.Stderr, "short write %s\n", to)
			return
		}
	}
}

func helpstring(cmd string) string {
	k, ok := cmds[cmd]
	if !ok {
		return "No help for "+cmd
	}
	return k.help
}

// Mount the given network name on mountpoint
func cmdmount(s []string) {
	repterr := true
	var opts uint32

	l := len(s)
	if l < 2 || l > 3 {
		fmt.Fprintf(os.Stderr, "%s\n", helpstring("mount"))
		return
	}
	
	if l > 2 {
		switch {
		case strings.ContainsRune(s[0], 'b'):
			opts = p.MBEFORE
		case strings.ContainsRune(s[0], 'a'):
			opts = p.MAFTER
		}
		if strings.ContainsRune(s[0], 'c') {
			opts |= p.MCREATE
		}
		if strings.ContainsRune(s[0], 'C') {
			opts |= p.MCACHE
		}
		if strings.ContainsRune(s[0], 'q') {
			repterr = false
		}
	s = s[1:]
	}

	c, err := chan9.Dial(s[0])
	if err != nil {
		if repterr {
			fmt.Fprintf(os.Stderr, "Error opening connection to %s: %s\n", s[0], err)
		}
		return
	}
	err = ns.Mount(c, nil, s[1], opts, "")
	if err != nil {
		if repterr {
			fmt.Fprintf(os.Stderr, "Error mounting connection to %s: %s\n", s[0], err)
		}
		c.Clunk(err)
		return
	}
}

// Mount the given dir on mountpoint
func cmdbind(s []string) {
	repterr := true
	var opts uint32

	l := len(s)
	if l < 2 || l > 3 {
		fmt.Fprintf(os.Stderr, "%s\n", helpstring("bind"))
		return
	}
	
	if l > 2 {
		switch {
		case strings.ContainsRune(s[0], 'b'):
			opts = p.MBEFORE
		case strings.ContainsRune(s[0], 'a'):
			opts = p.MAFTER
		}
		if strings.ContainsRune(s[0], 'c') {
			opts |= p.MCREATE
		}
		if strings.ContainsRune(s[0], 'C') {
			opts |= p.MCACHE
		}
		if strings.ContainsRune(s[0], 'q') {
			repterr = false
		}
	s = s[1:]
	}
	err := ns.Bind(s[0], s[1], opts)
	if err != nil {
		if repterr {
			fmt.Fprintf(os.Stderr, "Error binding %s to %s: %s\n", s[1], s[0], err)
		}
		return
	}
}

func cmdnetstat(s []string) {
	chan9.PrintClntList()
}

// List the mounts from/to the given dir.
func cmdlsmount(s []string) {
	if len(s) == 0 {
		ns.Mnt.PrintMnttab()
		return
	}

	if len(s) != 1 {
		fmt.Fprintf(os.Stderr, "%s\n", helpstring("lsmount"))
		return
	}
	parents, children, err := ns.LsMounts(s[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return
	}
	str := "Parents:"
	for _, pa := range parents {
		str = str+"\n\t"+pa
	}
	str = str+"\nChildren:"
	for _, ch := range children {
		str = str+"\n\t"+ch
	}
	fmt.Fprintf(os.Stdout, "%s\n", str)
}

// Remove the given mount.
func cmdumount(s []string) {
	if len(s) != 2 {
		fmt.Fprintf(os.Stderr, "%s\n", helpstring("umount"))
		return
	}
	err := ns.Umount(s[0], s[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return
	}
}

func cmdpwd(s []string) { fmt.Fprintf(os.Stdout, "/"+strings.Join(ns.Cwd.Cname,"/")+"\n") }

// Remove f from remote server
func rmone(fname chan9.Elemlist) {
	err := ns.FRemove(fname)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error in stat %s", err)
		return
	}
}
// Remove one or more files from the server
func cmdrm(s []string) {
	for _, f := range s {
		rmone(chan9.ParseName(f))
	}
}

// Print available commands
func cmdhelp(s []string) {
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

func cmdquit(s []string) { os.Exit(0) }

func cmd(cmd string) {
	ncmd := strings.Fields(cmd)
	if len(ncmd) <= 0 {
		return
	}
	v, ok := cmds[ncmd[0]]
	if ok == false {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", ncmd[0])
		return
	}
	v.fun(ncmd[1:])
	return
}

func interactive() {
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
				cmd(in[i])
			}
		}
	}
}

func main() {
	var err error
	var c *chan9.Clnt
	var file *chan9.File

	flag.Parse()

	naddr := *addr
	c, err = chan9.Dial(*addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to %s: %s\n", naddr, err)
		os.Exit(1)
	}

	if *ouser != "" {
		c.User = p.OsUsers.Uname2User(*ouser)
	}
	if *debug {
		c.Debuglevel = 1
	}
	if *debugall {
		c.Debuglevel = 2
	}

	ns, err = chan9.NSFromClnt(c, nil, p.MREPL|p.MCREATE, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error mounting %s: %s\n", naddr, err)
		os.Exit(1)
	}

	if file != nil {
		//process(c)
		fmt.Sprint(os.Stderr, "file reading unimplemented\n")
	} else if flag.NArg() > 0 {
		flags := flag.Args()
		for _, uc := range flags {
			cmd(uc)
		}
	} else {
		interactive()
	}

	return
}
