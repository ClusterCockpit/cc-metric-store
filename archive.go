package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

type ArchiveMetrics struct {
	Frequency int64   `json:"frequency"`
	Start     int64   `json:"start"`
	Data      []Float `json:"data"`
}

type ArchiveFile struct {
	From     int64                      `json:"from"`
	Metrics  map[string]*ArchiveMetrics `json:"metrics"`
	Children map[string]*ArchiveFile    `json:"children"`
}

// Metrics stored at the lowest 2 levels are not stored away (root and cluster)!
// On a per-host basis a new JSON file is created. I have no idea if this will scale.
// The good thing: Only a host at a time is locked, so this function can run
// in parallel to writes/reads.
func (m *MemoryStore) ToArchive(archiveRoot string, from, to int64) (int, error) {
	levels := make([]*level, 0)
	selectors := make([][]string, 0)
	m.root.lock.Lock()
	for sel1, l1 := range m.root.children {
		l1.lock.Lock()
		for sel2, l2 := range l1.children {
			levels = append(levels, l2)
			selectors = append(selectors, []string{sel1, sel2})
		}
		l1.lock.Unlock()
	}
	m.root.lock.Unlock()

	for i := 0; i < len(levels); i++ {
		dir := path.Join(archiveRoot, path.Join(selectors[i]...))
		err := levels[i].toArchive(dir, from, to)
		if err != nil {
			return i, err
		}
	}

	return len(levels), nil
}

func (l *level) toArchiveFile(from, to int64) (*ArchiveFile, error) {
	l.lock.Lock()
	defer l.lock.Unlock()

	retval := &ArchiveFile{
		From:     from,
		Metrics:  make(map[string]*ArchiveMetrics),
		Children: make(map[string]*ArchiveFile),
	}

	for metric, b := range l.metrics {
		data, start, _, err := b.read(from, to)
		if err != nil {
			return nil, err
		}

		retval.Metrics[metric] = &ArchiveMetrics{
			Frequency: b.frequency,
			Start:     start,
			Data:      data,
		}
	}

	for name, child := range l.children {
		val, err := child.toArchiveFile(from, to)
		if err != nil {
			return nil, err
		}

		retval.Children[name] = val
	}

	return retval, nil
}

func (l *level) toArchive(dir string, from, to int64) error {
	af, err := l.toArchiveFile(from, to)
	if err != nil {
		return err
	}

	filepath := path.Join(dir, fmt.Sprintf("%d.json", from))
	f, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
		if err == nil {
			f, err = os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY, 0644)
		}
	}
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)

	err = json.NewEncoder(bw).Encode(af)
	if err != nil {
		return err
	}

	return bw.Flush()
}

// Metrics stored at the lowest 2 levels are not loaded (root and cluster)!
// This function can only be called once and before the very first write or read.
// Unlike ToArchive, this function is NOT thread-safe.
func (m *MemoryStore) FromArchive(archiveRoot string, from int64) (int, error) {
	return m.root.fromArchive(archiveRoot, from)
}

func (l *level) loadFile(af *ArchiveFile) error {
	for name, metric := range af.Metrics {
		n := len(metric.Data)
		b := &buffer{
			frequency: metric.Frequency,
			start:     metric.Start,
			data:      metric.Data[0:n:n], // Space is wasted here :(
			prev:      nil,
			next:      nil,
		}

		prev, ok := l.metrics[name]
		if !ok {
			l.metrics[name] = b
		} else {
			if prev.start > b.start {
				return errors.New("wooops")
			}

			b.prev = prev
			prev.next = b
		}
		l.metrics[name] = b
	}

	for sel, childAf := range af.Children {
		child, ok := l.children[sel]
		if !ok {
			child = &level{
				metrics:  make(map[string]*buffer),
				children: make(map[string]*level),
			}
			l.children[sel] = child
		}

		err := child.loadFile(childAf)
		if err != nil {
			return err
		}
	}

	return nil
}

func (l *level) fromArchive(dir string, from int64) (int, error) {
	direntries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	jsonFiles := make([]fs.DirEntry, 0)
	filesLoaded := 0
	for _, e := range direntries {
		if e.IsDir() {
			child := &level{
				metrics:  make(map[string]*buffer),
				children: make(map[string]*level),
			}

			files, err := child.fromArchive(path.Join(dir, e.Name()), from)
			filesLoaded += files
			if err != nil {
				return filesLoaded, err
			}

			l.children[e.Name()] = child
		} else if strings.HasSuffix(e.Name(), ".json") {
			jsonFiles = append(jsonFiles, e)
		} else {
			return filesLoaded, errors.New("unexpected file in archive")
		}
	}

	files, err := findFiles(jsonFiles, from)
	if err != nil {
		return filesLoaded, err
	}

	for _, filename := range files {
		f, err := os.Open(path.Join(dir, filename))
		if err != nil {
			return filesLoaded, err
		}

		af := &ArchiveFile{}
		err = json.NewDecoder(bufio.NewReader(f)).Decode(af)
		if err != nil {
			return filesLoaded, err
		}

		err = l.loadFile(af)
		if err != nil {
			return filesLoaded, err
		}

		filesLoaded += 1
	}

	return filesLoaded, nil
}

// This will probably get very slow over time!
// A solution could be some sort of an index file in which all other files
// and the timespan they contain is listed.
func findFiles(direntries []fs.DirEntry, from int64) ([]string, error) {
	nums := map[string]int64{}
	for _, e := range direntries {
		ts, err := strconv.ParseInt(strings.TrimSuffix(e.Name(), ".json"), 10, 64)
		if err != nil {
			return nil, err
		}
		nums[e.Name()] = ts
	}

	sort.Slice(direntries, func(i, j int) bool {
		a, b := direntries[i], direntries[j]
		return nums[a.Name()] < nums[b.Name()]
	})

	filenames := make([]string, 0)
	for i := 0; i < len(direntries); i++ {
		e := direntries[i]
		ts1 := nums[e.Name()]

		if from <= ts1 || i == len(direntries)-1 {
			filenames = append(filenames, e.Name())
			continue
		}

		enext := direntries[i+1]
		ts2 := nums[enext.Name()]
		if ts1 < from && from < ts2 {
			filenames = append(filenames, e.Name())
		}
	}

	return filenames, nil
}
