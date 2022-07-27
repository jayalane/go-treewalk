// -*- tab-width: 2 -*-

package treewalk

import (
	"fmt"
	count "github.com/jayalane/go-counter"
	"os"
	"sync"
	"testing"
)

func TestMyJoin(t *testing.T) {
	a := [5]string{"", "", "", "", "."}
	b := myJoin(a[:], "/")
	if b != "." {
		fmt.Println("Got {", b, "} expected { . }")
		t.Fatal()
	}
}

func TestMyJoin2(t *testing.T) {
	a := [5]string{"..", "dedup", "", "", ""}
	b := myJoin(a[:], "/")
	if b != "../dedup" {
		fmt.Println("Got {", b, "} expected { ../dedup }")
		t.Fatal()
	}
}

func TestMyCopy(t *testing.T) {
	a := [maxPathDepth]string{"", "", "", "", ""}
	b := []string{".."}
	myCopy(&a, b)
	if a[0] != ".." {
		fmt.Println("Got {", a[0], "} expected { .. }")
		t.Fatal()
	}
}

func TestPrint(t *testing.T) {
	app := New("..", 2)
	gNum := [2]int{1, 5}
	app.SetNumWorkers(gNum[:])
	app.SetHandler(1, // files
		func(sp StringPath, chList []chan StringPath, wg *sync.WaitGroup) {
			fullPath := append(sp.Path[:], sp.Name)
			fn := myJoin(fullPath, "/")
			fi, err := os.Lstat(fn)
			if err != nil {
				app.log.La("Stat error on", fn, err)
				count.Incr("file-handler-stat-error")
				return
			}
			fmt.Println(fn, fi.ModTime())
			count.Incr("file-handler-ok")
		})
	app.Start()
	app.Wait()
}
