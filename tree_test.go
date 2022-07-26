// -*- tab-width: 2 -*-

package treewalk

import (
	"fmt"
	count "github.com/jayalane/go-counter"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestPrint(t *testing.T) {
	app := New(".", 2)
	app.SetHandler(1, // files
		func(sp StringPath, chList []chan StringPath, wg *sync.WaitGroup) {
			fullPath := append(sp.Path, sp.Name)
			fn := strings.Join(fullPath, "/")
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
