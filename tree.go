// -*- tab-width:2 -*-

package treewalk

import (
	"errors"
	"fmt"
	count "github.com/jayalane/go-counter"
	lll "github.com/jayalane/go-lll"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// MaxDepth is the greatest depth of layers you can have
const MaxDepth = 5

// maxSplits is the largest number of directory names that can be skipped
const maxSplits = 10

// StringPath is a string that's the current layer ID and the previous
// layers IDs needed to get to this one. e.g. name would be the file
// name and path would be the containing directories (for a file
// search, or name would be the object Key and the path would be the
// account ID and the bucket name (for S3)
type StringPath struct {
	Name string
	Path []string
}

// Callback is the thing called for each string read from a channel; it can do anything (print a file)
// or push more strings into a channel (if a directory element is another directory)
// if the callback wants to add more work, call t.SendOn(level, name, old StringPath)
type Callback func(sp StringPath)

// Treewalk keeps the state involved in walking thru a tree like dataflow
type Treewalk struct {
	firstString string
	numWorkers  []int
	cbs         []Callback
	chs         []chan StringPath
	skips       []string
	lock        *sync.RWMutex
	log         *lll.Lll
	wg          *sync.WaitGroup
	depth       int
}

// first a utility function to do joins that treats "" as nil
func myJoin(strs []string, delim string) string {
	res := ""
	seenPrev := false
	for _, s := range strs {
		if !seenPrev && s == "" {
			seenPrev = false
			continue
		}
		if s == "" {
			continue
		}
		if seenPrev {
			res = res + delim + s
		} else {
			res = res + s
		}
		seenPrev = true
	}
	return res
}

// New returns the context needed to start a treewalk
func New(firstString string, depth int) Treewalk {
	if depth > MaxDepth {
		s := fmt.Sprintln("MaxDepth is", MaxDepth, "depth", depth, "is too high")
		panic(s)
	}
	res := Treewalk{}
	res.firstString = firstString
	res.cbs = make([]Callback, depth)
	res.cbs[0] = nil // unneeded
	res.chs = make([]chan StringPath, depth)
	res.numWorkers = make([]int, depth)
	res.skips = make([]string, maxSplits)
	for i := 0; i < depth; i++ {
		res.chs[i] = make(chan StringPath, 1000000)
		res.numWorkers[i] = 5 // default?
	}
	res.depth = depth
	res.lock = &sync.RWMutex{}
	res.wg = &sync.WaitGroup{}
	log := lll.Init("Treewalk", "network") // should be settable
	res.log = &log
	return res
}

// SetLogLevel configures the underlying logging to either "network"
// (most logging) "state" "all" or "none"
func (t Treewalk) SetLogLevel(level string) {
	t.log.SetLevel(level)
}

// skipDir returns if a prospective dir should be skipped
// e.g. .snapshot on a NAC. Called from defaultDirHandle, not
// essential
func (t Treewalk) skipDir(dir string) bool {
	for _, x := range t.skips {
		if x == dir {
			return true
		}
	}
	return false
}

// defaultDirHandle is a default handler in the case this app is doing
// find type search on a filesystem
func (t Treewalk) defaultDirHandle(sp StringPath) {
	fullPath := append(sp.Path[:], sp.Name)
	fn := strings.Join(fullPath[:], "/")
	fn = filepath.Clean(fn)
	des, err := ReadDir(fn)
	if err != nil {
		t.log.La("Error on ReadDir", sp.Name, err)
		return
	}
	count.Incr("dir-handler-readdir-ok")
	for _, de := range des {
		t.log.Ln("Got a dirEntry", de.Name())
		count.Incr("dir-handler-dirent-got")
		if de.IsDir() {
			if t.skipDir(de.Name()) {
				t.log.Ls("Skipping", de.Name())
				count.Incr("dir-handler-dirent-skip")
				continue
			}
			t.SendOn(0, de.Name(), sp)
			count.Incr("dir-handler-dirent-got-dir")
		} else {
			t.SendOn(1, de.Name(), sp)
			count.Incr("dir-handler-dirent-got-not-dir")
		}
	}
}

// SendOn puts the new StringPath from name and old StringPath sp into
// the channel for the level  The channel isn't exposed so not every callback
// has to worry if the channel is full and starting more workers or whatever.
func (t Treewalk) SendOn(level int, name string, sp StringPath) {
	pathNewA := append(sp.Path[:], sp.Name)
	pathNewB := make([]string, len(pathNewA))
	copy(pathNewB, pathNewA)
	spNew := StringPath{name, pathNewB[:]}
	t.wg.Add(1)
	for {
		select {
		case t.chs[level] <- spNew:
			return
		default:
			// something to spawn more goroutines
			continue
		}
	}
}

// SetHandler takes a handler and a depth and saves the callback/handler
func (t Treewalk) SetHandler(level int, cb Callback) { // int before func for formatting prettiness
	t.lock.Lock()
	defer t.lock.Unlock()
	t.cbs[level] = cb
}

// SetNumWorkers overrides the number of go routines for each layer (default 5)
func (t Treewalk) SetNumWorkers(numWorkers []int) {
	if len(numWorkers) != t.depth {
		s := fmt.Sprintln("numWorkers length", len(numWorkers), "differs from depth", t.depth)
		panic(s)
	}
	t.lock.Lock()
	defer t.lock.Unlock()
	for i, n := range numWorkers {
		t.log.La("Setting layer", i, "to", n, "workers")
		t.numWorkers[i] = n
	}
}

// SetSkipDirs takes a slice of strings of directories to skip over in
// the default layer 0 handler
func (t *Treewalk) SetSkipDirs(skips []string) { // int before func for formatting prettiness
	t.lock.Lock()
	defer t.lock.Unlock()
	t.log.La("Setting skip list to", skips)
	t.skips = skips[:]
	t.log.La("Setting skip list to", t.skips)
}

// Start starts the go routines for the processing
func (t Treewalk) Start() {
	t.wg.Add(1)
	for i := 0; i < t.depth; i++ {
		for j := 0; j < t.numWorkers[i]; j++ {
			t.wg.Add(1)
			go func(layer int) {
				t.log.La("Starting go routine for tree walk depth", layer)
				for {
					select {
					case d := <-t.chs[layer]:
						t.log.Ln("Got a thing {", d.Name, "} layer", layer)
						if t.cbs[layer] != nil {
							t.cbs[layer](d)
						} else if layer == 0 {
							t.defaultDirHandle(d)
						} else {
							s := fmt.Sprintln("empty callback misconfigured")
							panic(s)
						}
						t.wg.Done()
					case <-time.After(3 * time.Second):
						t.log.La("Giving up on layer", layer, "after 3 seconds with no traffic")
						t.wg.Done()
						return
					}
				}
			}(i)
		}
	}
	t.wg.Add(1)
	var sp = StringPath{t.firstString, []string{}}
	t.chs[0] <- sp
	time.Sleep(1 * time.Second) // so there's some work done before exiting.
	t.wg.Done()
}

// Wait waits for the work to all finish
func (t Treewalk) Wait() {
	t.wg.Wait()
}

// ReadDir is a ReadDir with a timeout in case you are calling it on a
// big NAS that might never reply Default is 3600 seconds (1 hour);
// use ReadDirTimeout to tune this
func ReadDir(name string) ([]os.DirEntry, error) {
	r, e := ReadDirTimeout(name, time.Second*3600)
	return r, e
}

// ReadDirTimeout is a ReadDir with a timeout in case you are calling
// it on a big NAS that might never reply
func ReadDirTimeout(name string, t time.Duration) ([]os.DirEntry, error) {
	type res struct {
		de  []os.DirEntry
		err error
	}
	// start the DirEntry
	resCh := make(chan res, 2)
	go func(name string) {
		de, err := os.ReadDir(name)
		resCh <- res{de, err}
	}(name)
	// now wait for it
	select {
	case result := <-resCh:
		count.Incr("readdir-ok")
		return result.de, result.err
	case <-time.After(t):
		count.Incr("readdir-timeout")
		return nil, errors.New("Timeout on DirEntry")
	}
}
