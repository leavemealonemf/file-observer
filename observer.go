package observer

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	p "path"
	"strings"
	"sync"
	"time"
)

const (
	Create = iota
	Write
	Read
	Open
	Delete
)

type ReadFileInfo struct {
	modTime int64
	content []byte
}

type FileWrapper struct {
	f        fs.File
	filePath string
}

type Event struct {
	Op   int
	File string
}

type Observer struct {
	files   map[string]*FileWrapper
	dir     string
	filesMu sync.Mutex
	Events  chan Event
}

func (o *Observer) Add(path string) {
	fmt.Println("path", path)
	dir, err := os.Getwd()
	if err != nil {
		panic("failed to get root dir path")
	}

	var tempPath string

	if strings.Contains(path, dir) || (len(path) > 0 && path[0] == '/') {
		tempPath = path
	} else {
		tempPath = p.Join(dir, "/", path)
	}

	o.dir = dir

	fs := os.DirFS(tempPath)

	dirEntry, err := os.ReadDir(tempPath)
	if err != nil {
		panic(err)
	}

	for _, entry := range dirEntry {
		if entry.IsDir() {
			o.Add(p.Join(tempPath, "/", entry.Name()))
			continue
		}
		fName := entry.Name()
		f, err := fs.Open(fName)
		if err != nil {
			log.Println("failed to open", err)
		}
		o.register(fName, f, path)
	}
}

func (o *Observer) register(fName string, f fs.File, path string) {
	o.filesMu.Lock()
	defer o.filesMu.Unlock()
	_, exist := o.files[fName]
	if exist {
		return
	}

	fw := &FileWrapper{
		f:        f,
		filePath: p.Join(path, "/", fName),
	}

	o.files[fName] = fw
	log.Printf("file %s regiser by observer", fName)
	go startObserving(o, fName, fw)
}

func startObserving(o *Observer, fName string, fw *FileWrapper) {
	firstIn := true

	readState := func() (*ReadFileInfo, error) {
		f, err := os.Open(fw.filePath)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		bytes, err := io.ReadAll(f)
		if err != nil {
			return nil, err
		}

		stats, err := f.Stat()
		if err != nil {
			return nil, err
		}

		return &ReadFileInfo{
			content: bytes,
			modTime: stats.ModTime().Unix(),
		}, nil
	}

	prevState, err := readState()

	if err != nil {
		log.Println(err.Error())
		return
	}

	for {
		if firstIn {
			firstIn = false
			continue
		}

		state, err := readState()

		if err != nil {
			log.Println(err.Error())
			return
		}

		if string(state.content) != string(prevState.content) {
			prevState = state
			o.Events <- Event{
				Op:   Write,
				File: fName,
			}
		} else if state.modTime != prevState.modTime {
			prevState = state
			o.Events <- Event{
				Op:   Write,
				File: fName,
			}
		}

		time.Sleep(time.Second * 1)
	}
}

func NewObserver() *Observer {
	o := &Observer{
		files:  map[string]*FileWrapper{},
		Events: make(chan Event),
	}
	return o
}
