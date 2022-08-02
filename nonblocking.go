// -*- tab-width:2 -*-

package treewalk

import (
	"errors"
	count "github.com/jayalane/go-counter"
	"os"
	"time"
)

// this file is non-blocking versions of various os. calls that block or never
// return in the NFS case

// ReadDir is a ReadDir with a timeout in case you are calling it on a
// big NAS that might never reply Default is 60 seconds (1 minute);
// use ReadDirTimeout to tune this
func ReadDir(name string) ([]os.DirEntry, error) {
	r, e := ReadDirTimeout(name, time.Second*60)
	return r, e
}

// ReadDirTimeout is a ReadDir with a timeout in case you are calling
// it on a big NAS that might never reply
func ReadDirTimeout(name string, t time.Duration) ([]os.DirEntry, error) {
	type res struct {
		de  []os.DirEntry
		err error
	}
	// start the ReadDir
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
		return nil, errors.New("Timeout on ReadDir")
	}
}

// Open is an os.Open with a timeout in case you are calling it on a
// big NAS that might never reply Default is 60 seconds (1 minute);
// use OpenTimeout to tune the timeout
func Open(name string) (*os.File, error) {
	f, e := OpenTimeout(name, time.Second*60)
	return f, e
}

// OpenTimeout is an fs.Open  with a timeout in case you are calling
// it on a big NAS that might never reply
func OpenTimeout(name string, t time.Duration) (*os.File, error) {
	type res struct {
		f   *os.File
		err error
	}
	// start the Open
	resCh := make(chan res, 2)
	go func(name string) {
		f, err := os.Open(name)
		resCh <- res{f, err}
	}(name)
	// now wait for it
	select {
	case result := <-resCh:
		count.Incr("open-ok")
		return result.f, result.err
	case <-time.After(t):
		count.Incr("open-timeout")
		return nil, errors.New("Timeout on Open")
	}
}

// Lstat is an os.Lstat with a timeout. Default is 60 seconds (1 minute);
// use LstatTimeout to tune the timeout
func Lstat(name string) (os.FileInfo, error) {
	fi, e := LstatTimeout(name, time.Second*60)
	return fi, e
}

// LstatTimeout is an os.Lstat with a timeout parameter. Tmed out
// calls will leave a go routine and OS thread around.
func LstatTimeout(name string, t time.Duration) (os.FileInfo, error) {
	type res struct {
		fi  os.FileInfo
		err error
	}
	// start the Lstat
	resCh := make(chan res, 2)
	go func(name string) {
		fi, err := os.Lstat(name)
		resCh <- res{fi, err}
	}(name)
	select {
	case result := <-resCh:
		count.Incr("lstat-ok")
		return result.fi, result.err
	case <-time.After(t):
		count.Incr("lstat-timeout")
		return nil, errors.New("Timeout on Lstat")
	}
}
