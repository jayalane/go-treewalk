// -*- tab-width: 2 -*-

package treewalk

import (
	"fmt"
	count "github.com/jayalane/go-counter"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// had trouble with get //root/.bashrc////////// style from this.
func TestMyJoin(t *testing.T) {
	a := [5]string{"", "", "", "", "."}
	b := myJoin(a[:], "/")
	if b != "." {
		fmt.Println("Got {", b, "} expected { . }")
		t.Fatal()
	}
}

// even after that passed is was doing dedup.. not ../dedup
func TestMyJoin2(t *testing.T) {
	a := [5]string{"..", "dedup", "", "", ""}
	b := myJoin(a[:], "/")
	if b != "../dedup" {
		fmt.Println("Got {", b, "} expected { ../dedup }")
		t.Fatal()
	}
}

// a full run of the simple app on ../
// no validation
func TestPrint(t *testing.T) {
	count.InitCounters()
	app := New("/home", 2)
	gNum := [2]int64{1, 5}
	app.SetNumWorkers(gNum[:])
	app.SetLogLevel("state")
	testDir := []string{".git"} // skips .git
	app.SetSkipDirs(testDir)
	app.SetHandler(1, // files
		func(sp StringPath) {
			fullPath := append(sp.Path[:], sp.Name)
			fn := strings.Join(fullPath, "/")
			var fi fs.FileInfo
			var err error
			de, ok := sp.Value.(fs.DirEntry)
			if ok {
				fi, err = de.Info()
				count.Incr("Used de")
			} else {
				count.Incr("Used Lstat")
				fi, err = Lstat(fn)
			}
			if err != nil {
				app.log.La("Stat error on", fn, err)
				count.Incr("file-handler-stat-error")
				return
			}
			isSymLink := fi.Mode()&os.ModeSymlink == os.ModeSymlink
			if isSymLink { // the logic specific to this app
				fmt.Println("Is Symlink")
			}
			fmt.Println(fn, fi.ModTime())
			count.Incr("file-handler-ok")
		})
	app.Start()
	app.Wait()
	count.LogCounters()
}
