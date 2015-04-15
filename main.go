package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/bnagy/francis"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const MAXLEN = 10 * 1024 * 1024 // 10 MB

var (
	flagFn      *string = flag.String("fn", ".cur_input", "filename to use")
	flagDestDir *string = flag.String("dest", "", "directory to use to stage tests to disk")
	flagSrcDir  *string = flag.String("src", "", "directory with test corpus")
	flagServer  *string = flag.String("server", "", "remote radamsa server to use for tests")
	flagFix     *string = flag.String("fix", "", "Unix socket to use to fix tests")
	flagTimeout *int    = flag.Int("t", -1, "timeout in secs for app under test")
	// flagWorkers  *int    = flag.Int("workers", 1, "Number of concurrent workers")
)

func sleepyConnect(dest string) (s net.Conn, err error) {
	zzz := 1 * time.Millisecond
	for {
		if zzz > 1*time.Second {
			err = fmt.Errorf("connection to server timed out.")
			return
		}
		time.Sleep(zzz)
		s, err = net.Dial("tcp", dest)
		if err == nil {
			return
		}
		zzz *= 2
	}
}

func readNetString(r *bufio.Reader) ([]byte, error) {

	lenBytes, err := r.ReadBytes(':')
	if err != nil {
		return []byte{}, err
	}
	i, err := strconv.ParseInt(string(lenBytes[:len(lenBytes)-1]), 10, 0)
	dataLen := int(i) // 0 on error, safe to check this first
	if dataLen > MAXLEN {
		err = fmt.Errorf("Proposed Length > MAXLEN")
		return []byte{}, err
	}
	if err != nil {
		err = fmt.Errorf("Error Parsing Length %#v", lenBytes)
		return []byte{}, err
	}

	data := make([]byte, dataLen)
	_, err = io.ReadFull(r, data)
	if err != nil {
		err = fmt.Errorf("Error reading data")
		return []byte{}, err
	}
	b, err := r.ReadByte()
	if err != nil || b != ',' {
		err = fmt.Errorf("Missing terminator")
		return []byte{}, err
	}

	return data, nil
}

func stageTest(raw []byte) {
	err := ioutil.WriteFile(path.Join(*flagDestDir, *flagFn), raw, 0600)
	if err != nil {
		log.Fatalf("[SAD] failed to create test file: %s", err)
	}
}

func getTest() ([]byte, []byte) {

	conn, err := net.Dial("tcp", "127.0.0.1:4141")
	if err != nil {
		log.Fatalf("[SAD] Unable to connect to radamsa server: %s", err)
	}

	hsh := sha1.New()
	tee := io.TeeReader(conn, hsh)
	raw, err := ioutil.ReadAll(tee)

	if err != nil {
		log.Fatalf("[SAD] Error reading from server: %s", err)
	}
	return raw, hsh.Sum(nil)
}

func saveTest(fn string, raw []byte) {
	err := ioutil.WriteFile(path.Join(*flagDestDir, "crashes", fn), raw, 0600)
	if err != nil {
		log.Printf("[SUPER SAD] failed to write crashfile!: %s\n", err)
		hex.Dump(raw)
		log.Fatalf("[SUPER SAD] (that hexdump was the last test)\n")
	}
}

func fixTest(t []byte, conn net.Conn, rd *bufio.Reader) ([]byte, error) {

	s := fmt.Sprintf("%d:%s,", len(t), string(t))
	conn.Write([]byte(s))
	fixed, err := readNetString(rd)
	if err != nil {
		return []byte{}, err
	}

	return fixed, nil
}

func main() {

	flag.Usage = func() {
		fmt.Fprintf(
			os.Stderr,
			"  Usage: %s -src /path/to/corpus -dest /path/to/workdir -- /path/to/target -infile @@ -foo -quux\n"+
				"  OR: %s -server 192.168.1.1:4141 -dest /path/to/workdir -- /path/to/target -infile @@ -foo -quux\n",
			path.Base(os.Args[0]),
			path.Base(os.Args[0]),
		)
		fmt.Fprintf(os.Stderr, "  ( @@ will be substituted for each testfile )\n\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	log.Printf("%s - performing startup checks...\n", os.Args[0])

	testCmd := flag.Args()
	if len(testCmd) < 2 {
		log.Fatalf("[SAD] Minimum target command is: /path/to/target @@\n")
		flag.Usage()
		os.Exit(1)
	}

	if len(*flagServer) != 0 && len(*flagSrcDir) != 0 {
		log.Fatalf("[SAD] -src and -server cannot be used together.\n")
	}
	if len(*flagServer) == 0 && len(*flagSrcDir) == 0 {
		log.Fatalf("[SAD] need a corpus directory (-src) or a radamsa server address (-server)\n")
	}

	srvAddr := ""

	if len(*flagSrcDir) > 0 {
		// Sanity checks on the source directory
		fi, err := os.Stat(*flagSrcDir)
		if err != nil {
			log.Fatalf("[SAD] unable to open -src dir: %s", err)
		}
		if !fi.IsDir() {
			log.Fatalf("[SAD] -src is not a directory.")
		}
		if files, _ := filepath.Glob(path.Join(*flagSrcDir, "*")); len(files) < 1 {
			log.Printf("[WARNING] no files in source directory!\n")
		}
		log.Printf("[HAPPY] source dir looks ok...\n")

		// Start and test the local radamsa server
		radamsa := exec.Command("radamsa", "-n", "inf", "-o", ":4141", "-r", *flagSrcDir)
		err = radamsa.Start()
		if err != nil {
			log.Fatalf("[SAD] Unable to launch radamsa server: %s", err)
		}
		defer radamsa.Process.Kill()
		srvAddr = "127.0.0.1:4141"

	} else {
		srvAddr = *flagServer
	}

	conn, err := sleepyConnect(srvAddr)
	if err != nil {
		log.Fatalf("[SAD] Unable to connect to radamsa server: %s", err)
	}
	_, err = ioutil.ReadAll(conn)
	if err != nil {
		log.Fatalf("[SAD] Error reading from server: %s", err)
	}
	log.Printf("[HAPPY] radamsa server is running...\n")

	// Sanity checks on the dest dir
	fi, err := os.Stat(*flagDestDir)
	if err == nil && !fi.IsDir() {
		log.Fatalf("[SAD] -dest is not a directory.")
	}
	if err != nil {
		// Make the crashdir at the same time
		err = os.MkdirAll(path.Join(*flagDestDir, "crashes"), 0700)
		if err != nil {
			log.Fatalf("[SAD] failed to create -dest: %s", err)
		}
	}
	err = ioutil.WriteFile(path.Join(*flagDestDir, *flagFn), []byte("test"), 0600)
	if err != nil {
		log.Fatalf("[SAD] failed to create test file: %s", err)
	}
	log.Printf("[HAPPY] dest dir looks ok...\n")

	// make sure there's at least one substitute marker
	sub := 0
	for i, elem := range testCmd {
		if elem == "@@" {
			testCmd[i] = path.Join(*flagDestDir, *flagFn)
			sub++
		}
	}
	if sub == 0 {
		log.Fatalf("[SAD] No substitute markers ( @@ ) in supplied command")
	}
	log.Printf("[CALM] Will be fuzzing: %s\n", strings.Join(testCmd, " "))

	// test the fix sock, if given
	var fixReader *bufio.Reader
	var fixConn net.Conn
	if *flagFix != "" {
		fixConn, err = net.Dial("unix", *flagFix)
		if err != nil {
			log.Fatalf("[SAD] Unable to dial fix socket: %s", err)
		}
		fixReader = bufio.NewReader(fixConn)
	}

	francis := &francis.Engine{*flagTimeout}
	log.Printf("[HAPPY] everything looks good. Let's go!\n")

	mark := time.Now()
	count := 0
	timer := time.Tick(30 * time.Second)
	go func() {
		for {
			<-timer
			elapsed := (time.Since(mark) / time.Second) * time.Second // truncate to 1s resolution
			fmt.Printf("\r[CALM] %d tests in %s (%.2f / s) %.20s", count, elapsed, float64(count)/float64(elapsed/time.Second), " ")
		}
	}()

	for {

		test, sha := getTest()
		if len(test) > MAXLEN {
			continue
		}
		if fixConn != nil {
			test, err = fixTest(test, fixConn, fixReader)
			if err != nil {
				log.Fatalf("[SAD] failed to fix test: %s", err)
			}
		}
		stageTest(test)

		count++

		ci, err := francis.Run(testCmd)
		if err == nil {
			// this is backasswards for this application. For the triage tool
			// err meant there was no crash.
			log.Printf("[HAPPY] Crash! - %s", ci.Extra[0])
			saveTest(fmt.Sprintf("%s.raw", hex.EncodeToString(sha)), test)
		}

	}

}
