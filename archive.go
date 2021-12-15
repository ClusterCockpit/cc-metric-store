package main

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type CheckpointMetrics struct {
	Frequency int64   `json:"frequency"`
	Start     int64   `json:"start"`
	Data      []Float `json:"data"`
}

type CheckpointFile struct {
	From     int64                         `json:"from"`
	Metrics  map[string]*CheckpointMetrics `json:"metrics"`
	Children map[string]*CheckpointFile    `json:"children"`
}

var ErrNoNewData error = errors.New("all data already archived")

// Metrics stored at the lowest 2 levels are not stored away (root and cluster)!
// On a per-host basis a new JSON file is created. I have no idea if this will scale.
// The good thing: Only a host at a time is locked, so this function can run
// in parallel to writes/reads.
func (m *MemoryStore) ToCheckpoint(dir string, from, to int64) (int, error) {
	levels := make([]*level, 0)
	selectors := make([][]string, 0)
	m.root.lock.RLock()
	for sel1, l1 := range m.root.children {
		l1.lock.RLock()
		for sel2, l2 := range l1.children {
			levels = append(levels, l2)
			selectors = append(selectors, []string{sel1, sel2})
		}
		l1.lock.RUnlock()
	}
	m.root.lock.RUnlock()

	n := 0
	for i := 0; i < len(levels); i++ {
		dir := path.Join(dir, path.Join(selectors[i]...))
		err := levels[i].toCheckpoint(dir, from, to, m)
		if err != nil {
			if err == ErrNoNewData {
				continue
			}

			return i, err
		}

		n += 1
	}

	return 0, nil
}

func (l *level) toCheckpointFile(from, to int64, m *MemoryStore) (*CheckpointFile, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	retval := &CheckpointFile{
		From:     from,
		Metrics:  make(map[string]*CheckpointMetrics),
		Children: make(map[string]*CheckpointFile),
	}

	for metric, minfo := range m.metrics {
		b := l.metrics[minfo.offset]
		if b == nil {
			continue
		}

		allArchived := true
		b.iterFromTo(from, to, func(b *buffer) error {
			if !b.archived {
				allArchived = false
			}
			return nil
		})

		if allArchived {
			continue
		}

		data := make([]Float, (to-from)/b.frequency+1)
		data, start, end, err := b.read(from, to, data)
		if err != nil {
			return nil, err
		}

		for i := int((end - start) / b.frequency); i < len(data); i++ {
			data[i] = NaN
		}

		retval.Metrics[metric] = &CheckpointMetrics{
			Frequency: b.frequency,
			Start:     start,
			Data:      data,
		}
	}

	for name, child := range l.children {
		val, err := child.toCheckpointFile(from, to, m)
		if err != nil {
			return nil, err
		}

		if val != nil {
			retval.Children[name] = val
		}
	}

	if len(retval.Children) == 0 && len(retval.Metrics) == 0 {
		return nil, nil
	}

	return retval, nil
}

func (l *level) toCheckpoint(dir string, from, to int64, m *MemoryStore) error {
	cf, err := l.toCheckpointFile(from, to, m)
	if err != nil {
		return err
	}

	if cf == nil {
		return ErrNoNewData
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

	if err = json.NewEncoder(bw).Encode(cf); err != nil {
		return err
	}

	return bw.Flush()
}

// Metrics stored at the lowest 2 levels are not loaded (root and cluster)!
// This function can only be called once and before the very first write or read.
// Different host's data is loaded to memory in parallel.
func (m *MemoryStore) FromCheckpoint(dir string, from int64) (int, error) {
	workers := runtime.NumCPU() / 2
	work := make(chan [2]string, workers)
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(workers + 1)

	go func() {
		defer close(work)
		defer wg.Done()

		clustersDir, err := os.ReadDir(dir)
		if err != nil {
			errs <- err
			return
		}

		for _, clusterDir := range clustersDir {
			if !clusterDir.IsDir() {
				errs <- errors.New("expected only directories at first level of checkpoints/ directory")
				return
			}

			hostsDir, err := os.ReadDir(filepath.Join(dir, clusterDir.Name()))
			if err != nil {
				errs <- err
				return
			}

			for _, hostDir := range hostsDir {
				if !hostDir.IsDir() {
					errs <- errors.New("expected only directories at second level of checkpoints/ directory")
					return
				}

				work <- [2]string{clusterDir.Name(), hostDir.Name()}
			}
		}
	}()

	loadedFiles := int32(0)

	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			for host := range work {
				lvl := m.root.findLevelOrCreate(host[:], len(m.metrics))
				n, err := lvl.fromCheckpoint(filepath.Join(dir, host[0], host[1]), from, m)
				if err != nil {
					errs <- err
					return
				}
				atomic.AddInt32(&loadedFiles, int32(n))
			}
		}()
	}

	wg.Wait()
	var err error = nil
	select {
	case e := <-errs:
		err = e
	default:
	}
	return int(loadedFiles), err
}

func (l *level) loadFile(cf *CheckpointFile, m *MemoryStore) error {
	for name, metric := range cf.Metrics {
		n := len(metric.Data)
		b := &buffer{
			frequency: metric.Frequency,
			start:     metric.Start,
			data:      metric.Data[0:n:n], // Space is wasted here :(
			prev:      nil,
			next:      nil,
			archived:  true,
		}
		b.close()

		minfo, ok := m.metrics[name]
		if !ok {
			return errors.New("Unkown metric: " + name)
		}

		prev := l.metrics[minfo.offset]
		if prev == nil {
			l.metrics[minfo.offset] = b
		} else {
			if prev.start > b.start {
				return errors.New("wooops")
			}

			b.prev = prev
			prev.next = b
		}
		l.metrics[minfo.offset] = b
	}

	if len(cf.Children) > 0 && l.children == nil {
		l.children = make(map[string]*level)
	}

	for sel, childCf := range cf.Children {
		child, ok := l.children[sel]
		if !ok {
			child = &level{
				metrics:  make([]*buffer, len(m.metrics)),
				children: nil,
			}
			l.children[sel] = child
		}

		if err := child.loadFile(childCf, m); err != nil {
			return err
		}
	}

	return nil
}

func (l *level) fromCheckpoint(dir string, from int64, m *MemoryStore) (int, error) {
	direntries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}

		return 0, err
	}

	jsonFiles := make([]fs.DirEntry, 0)
	filesLoaded := 0
	for _, e := range direntries {
		if e.IsDir() {
			child := &level{
				metrics:  make([]*buffer, len(m.metrics)),
				children: make(map[string]*level),
			}

			files, err := child.fromCheckpoint(path.Join(dir, e.Name()), from, m)
			filesLoaded += files
			if err != nil {
				return filesLoaded, err
			}

			l.children[e.Name()] = child
		} else if strings.HasSuffix(e.Name(), ".json") {
			jsonFiles = append(jsonFiles, e)
		} else {
			return filesLoaded, errors.New("unexpected file: " + dir + "/" + e.Name())
		}
	}

	files, err := findFiles(jsonFiles, from, true)
	if err != nil {
		return filesLoaded, err
	}

	for _, filename := range files {
		f, err := os.Open(path.Join(dir, filename))
		if err != nil {
			return filesLoaded, err
		}

		br := bufio.NewReader(f)
		cf := &CheckpointFile{}
		if err = json.NewDecoder(br).Decode(cf); err != nil {
			return filesLoaded, err
		}

		if err = l.loadFile(cf, m); err != nil {
			return filesLoaded, err
		}

		if err = f.Close(); err != nil {
			return filesLoaded, err
		}

		filesLoaded += 1
	}

	return filesLoaded, nil
}

// This will probably get very slow over time!
// A solution could be some sort of an index file in which all other files
// and the timespan they contain is listed.
func findFiles(direntries []fs.DirEntry, t int64, findMoreRecentFiles bool) ([]string, error) {
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

		if findMoreRecentFiles && t <= ts1 || i == len(direntries)-1 {
			filenames = append(filenames, e.Name())
			continue
		}

		enext := direntries[i+1]
		ts2 := nums[enext.Name()]

		if findMoreRecentFiles {
			if ts1 < t && t < ts2 {
				filenames = append(filenames, e.Name())
			}
		} else {
			if ts2 < t {
				filenames = append(filenames, e.Name())
			}
		}
	}

	return filenames, nil
}

// ZIP all checkpoint files older than `from` together and write them to the `archiveDir`,
// deleting them from the `checkpointsDir`.
func ArchiveCheckpoints(checkpointsDir, archiveDir string, from int64) error {
	entries1, err := os.ReadDir(checkpointsDir)
	if err != nil {
		return err
	}

	for _, de1 := range entries1 {
		entries2, err := os.ReadDir(filepath.Join(checkpointsDir, de1.Name()))
		if err != nil {
			return err
		}

		for _, de2 := range entries2 {
			cdir := filepath.Join(checkpointsDir, de1.Name(), de2.Name())
			adir := filepath.Join(archiveDir, de1.Name(), de2.Name())
			if err := archiveCheckpoints(cdir, adir, from); err != nil {
				return err
			}
		}
	}

	return nil
}

// Helper function for `ArchiveCheckpoints`.
func archiveCheckpoints(dir string, archiveDir string, from int64) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	files, err := findFiles(entries, from, false)
	if err != nil {
		return err
	}

	filename := filepath.Join(archiveDir, fmt.Sprintf("%d.zip", from))
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(archiveDir, 0755)
		if err == nil {
			f, err = os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0644)
		}
	}
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)
	zw := zip.NewWriter(bw)

	for _, jsonFile := range files {
		filename := filepath.Join(dir, jsonFile)
		r, err := os.Open(filename)
		if err != nil {
			return err
		}

		w, err := zw.Create(jsonFile)
		if err != nil {
			return err
		}

		if _, err = io.Copy(w, r); err != nil {
			return err
		}

		if err = os.Remove(filename); err != nil {
			return err
		}
	}

	if err = zw.Close(); err != nil {
		return err
	}

	if err = bw.Flush(); err != nil {
		return err
	}

	return nil
}
