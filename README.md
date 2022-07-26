Go TreeWalk scaffold
====================

This package provides the scaffolding for an app (cli?) that iterates
over a tree-like sort of problem domain, e.g.

"find all the files with 0 length in this path" (directory => more
directories or files)

"find all the files from Log4J v2 in this NFS filer, even if they are
in a jar file (directory => more directories, files, or jar files)

"find all objects in this S3 bucket with such and such property"
(accounts => buckets => keys => objects).

The test file shows how a main.go could call it.  

New specifies the initial string to put into the layer 0 channel to
kick start it and the number of layers.  (e.g. 2 for the directories/files search)

SetHandler specifies a function to run on each thing put into the
layer N channel, so for the below example, it's calls Lstat and prints the filename


```
	app := New(".", 2)
	app.SetHandler(1, // files
		func(sp StringPath, chList []chan StringPath, wg *sync.WaitGroup) {
			fullPath := append(sp.Path, sp.Name)
			fn := strings.Join(fullPath, "/")
			fi, err := os.Lstat(fn)
			if err != nil {
				app.log.La("Stat error on", name, err)
				count.Incr("file-handler-stat-error")
				return
			}
			fmt.Println(fn, fi.ModTime())
			count.Incr("file-handler-ok")
		})
	app.Start()
	app.Wait()
```

It doesn't print anything, that's up to the provided call backs.
There's a default callback for layer 0, where it assumes it needs to
call ReadDir and stuff all the dirs back into the layer 0 channel and
all the files into the layer 1 channel.


