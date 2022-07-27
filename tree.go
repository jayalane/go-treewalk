// -*- tab-width:2 -*-

package treewalk

import (
	"fmt"
	count "github.com/jayalane/go-counter"
	lll "github.com/jayalane/go-lll"
	"os"
	"sync"
	"time"
)

// MaxDepth is the greatest depth of layers you can have
const MaxDepth = 5

// maxPathDepth is the largest number of path segments that will be seen for th
// default handler
const maxPathDepth = 50

// maxSplits is the largest number of directory names that can be skipped
const maxSplits = 10

// StringPath is a string that's the current layer ID and the previous
// layers IDs needed to get to this one. e.g. name would be the file
// name and path would be the containing directories (for a file
// search, or name would be the object Key and the path would be the
// account ID and the bucket name (for S3)
type StringPath struct {
	Name string
	Path [maxPathDepth]string
}

// Callback is the thing called for each string read from a channel; it can do anything (print a file)
// or push more strings into a channel (if a directory element is another directory)
// if the callback adds strings to the channel, it should call wg.Add(1); it should not call wg.Done()
type Callback func(StringPath, []chan StringPath, *sync.WaitGroup)

// Treewalk keeps the state involved in walking thru a tree like dataflow
type Treewalk struct {
	firstString string
	numWorkers  []int
	cbs         []Callback
	chs         []chan StringPath
	skips       []string
	lock        *sync.RWMutex
	log         lll.Lll
	wg          *sync.WaitGroup
	depth       int
}

// first a utility function to do joins on string lists (not slices)
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

// first a utility function to do joins on string lists (not slices)
func myCopy(dst *[maxPathDepth]string, src []string) {
	j := 0
	for _, s := range src {
		if s != "" {
			(*dst)[j] = s
			j = j + 1
		}
		if j > maxPathDepth {
			fmt.Println("dst is", dst, "src is", src)
			s := fmt.Sprintln("src len", len(src), "is bigger than maxPathDepth", maxPathDepth)
			panic(s)
		}
	}
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
	res.log = lll.Init("Treewalk", "network") // should be settable
	return res
}

func (t Treewalk) skipDir(dir string) bool {
	t.log.La("Checking", dir, "for skipping")
	t.log.La("List of skips is", t.skips)
	for _, x := range t.skips {
		t.log.La("Got x", x)
		if x == dir {
			return true
		}
	}
	return false
}

func (t Treewalk) defaultDirHandle(sp StringPath, chs []chan StringPath, wg *sync.WaitGroup) {
	fullPath := append(sp.Path[:], sp.Name)
	fn := myJoin(fullPath[:], "/")
	des, err := os.ReadDir(fn)
	if err != nil {
		t.log.La("Error on ReadDir", sp.Name, err)
		return
	}
	count.Incr("dir-handler-readdir-ok")
	for _, de := range des {
		t.log.Ln("Got a dirEntry", de)
		count.Incr("dir-handler-dirent-got")
		pathNew := append(sp.Path[:], sp.Name)
		var tmp [maxPathDepth]string
		myCopy(&tmp, pathNew)
		if de.IsDir() {
			count.Incr("dir-handler-dirent-got-dir")
			if t.skipDir(de.Name()) {
				t.log.Ls("Skipping", de.Name())
				count.Incr("dir-handler-dirent-skip")
				continue
			}
			wg.Add(1)
			spNew := StringPath{de.Name(), tmp}
			chs[0] <- spNew
		} else {
			count.Incr("dir-handler-dirent-got-not-dir")
			wg.Add(1)
			spNew := StringPath{de.Name(), tmp}
			chs[1] <- spNew
		}
	}
}

// SetHandler takes a handler and a depth
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

// SetSkipDirs takes a slice of strings of directories to skip over
// in the default layer 0 handler
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
						t.log.Ln("Got a thing {", d, "} layer", layer)
						if t.cbs[layer] != nil {
							t.cbs[layer](d, t.chs, t.wg)
						} else if layer == 0 {
							t.defaultDirHandle(d, t.chs, t.wg)
						}
						t.wg.Done()
					case <-time.After(3 * time.Second):
						t.log.La("Giving up on layer", layer, "after 1 minute with no traffic")
						t.wg.Done()
						return
					}
				}
			}(i)
		}
	}
	t.wg.Add(1)
	var sp = StringPath{t.firstString, [maxPathDepth]string{}}
	t.chs[0] <- sp
	time.Sleep(1 * time.Second)
	t.wg.Done()
}

// Wait waits for the work to all finish
func (t Treewalk) Wait() {
	t.wg.Wait()
}
