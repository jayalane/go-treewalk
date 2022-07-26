// -*- tab-width:2 -*-

package treewalk

import (
	count "github.com/jayalane/go-counter"
	lll "github.com/jayalane/go-lll"
	"os"
	"strings"
	"sync"
	"time"
)

// StringPath is a string that's the current layer ID and the previous
// layers IDs needed to get to this one. e.g. name would be the file
// name and path would be the containing directories (for a file
// search, or name would be the object Key and the path would be the
// account ID and the bucket name (for S3)
type StringPath struct {
	name string
	path []string
}

// Callback is the thing called for each string read from a channel; it can do anything (print a file)
// or push more strings into a channel (if a directory element is another directory)
// if the callback adds strings to the channel, it should call wg.Add(1); it should not call wg.Done()
type Callback func(StringPath, []chan StringPath, *sync.WaitGroup)

// Treewalk keeps the state involved in walking thru a tree like dataflow
type Treewalk struct {
	firstString string
	cbs         []Callback
	chs         []chan StringPath
	lock        *sync.RWMutex
	log         lll.Lll
	wg          *sync.WaitGroup
	depth       int
}

// New returns the context needed to start a treewalk
func New(firstString string, depth int) Treewalk {
	res := Treewalk{}
	res.firstString = firstString
	res.cbs = make([]Callback, depth)
	res.cbs[0] = nil // unneeded
	res.chs = make([]chan StringPath, depth)
	for i := 0; i < depth; i++ {
		res.chs[i] = make(chan StringPath, 1000000)
	}
	res.depth = depth
	res.lock = &sync.RWMutex{}
	res.wg = &sync.WaitGroup{}
	res.log = lll.Init("Treewalk", "network") // should be settable
	return res
}

func (t Treewalk) defaultDirHandle(sp StringPath, chs []chan StringPath, wg *sync.WaitGroup) {
	fullPath := append(sp.path, sp.name)
	des, err := os.ReadDir(strings.Join(fullPath, "/"))
	if err != nil {
		t.log.La("Error on ReadDir", sp.name, err)
		return
	}
	count.Incr("dir-handler-readdir-ok")
	for _, de := range des {
		t.log.Ln("Got a dirEntry", de)
		count.Incr("dir-handler-dirent-got")
		if de.IsDir() {
			count.Incr("dir-handler-dirent-got-dir")
			wg.Add(1)
			pathNew := append(sp.path, sp.name)
			spNew := StringPath{de.Name(), pathNew}
			chs[0] <- spNew
		} else {
			count.Incr("dir-handler-dirent-got-not-dir")
			wg.Add(1)
			pathNew := append(sp.path, sp.name)
			spNew := StringPath{de.Name(), pathNew}
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

// Start starts the go routines for the processing
func (t Treewalk) Start() {
	t.wg.Add(1)
	for i := 0; i < t.depth; i++ {
		for j := 0; j < 1; j++ {
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
	var sp = StringPath{t.firstString, []string{}}
	t.chs[0] <- sp
	time.Sleep(1 * time.Second)
	t.wg.Done()
}

// Wait waits for the work to all finish
func (t Treewalk) Wait() {
	t.wg.Wait()
}
