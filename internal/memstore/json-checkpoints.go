package memstore

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ClusterCockpit/cc-metric-store/internal/types"
)

// Whenever changed, update MarshalJSON as well!
type JSONCheckpointMetrics struct {
	Frequency int64         `json:"frequency"`
	Start     int64         `json:"start"`
	Data      []types.Float `json:"data"`
}

// As `Float` implements a custom MarshalJSON() function,
// serializing an array of such types has more overhead
// than one would assume (because of extra allocations, interfaces and so on).
func (cm *JSONCheckpointMetrics) MarshalJSON() ([]byte, error) {
	buf := make([]byte, 0, 128+len(cm.Data)*8)
	buf = append(buf, `{"frequency":`...)
	buf = strconv.AppendInt(buf, cm.Frequency, 10)
	buf = append(buf, `,"start":`...)
	buf = strconv.AppendInt(buf, cm.Start, 10)
	buf = append(buf, `,"data":[`...)
	for i, x := range cm.Data {
		if i != 0 {
			buf = append(buf, ',')
		}
		if x.IsNaN() {
			buf = append(buf, `null`...)
		} else {
			buf = strconv.AppendFloat(buf, float64(x), 'f', 1, 32)
		}
	}
	buf = append(buf, `]}`...)
	return buf, nil
}

type JSONCheckpointFile struct {
	From     int64                             `json:"from"`
	To       int64                             `json:"to"`
	Metrics  map[string]*JSONCheckpointMetrics `json:"metrics"`
	Children map[string]*JSONCheckpointFile    `json:"children"`
}

var ErrNoNewData error = errors.New("all data already archived")

var NumWorkers int = 4

func init() {
	maxWorkers := 10
	NumWorkers = runtime.NumCPU()/2 + 1
	if NumWorkers > maxWorkers {
		NumWorkers = maxWorkers
	}
}

// Metrics stored at the lowest 2 levels are not stored away (root and cluster)!
// On a per-host basis a new JSON file is created. I have no idea if this will scale.
// The good thing: Only a host at a time is locked, so this function can run
// in parallel to writes/reads.
func (m *MemoryStore) ToJSONCheckpoint(dir string, from, to int64) (int, error) {
	levels := make([]*Level, 0)
	selectors := make([][]string, 0)
	m.root.lock.RLock()
	for sel1, l1 := range m.root.sublevels {
		l1.lock.RLock()
		for sel2, l2 := range l1.sublevels {
			levels = append(levels, l2)
			selectors = append(selectors, []string{sel1, sel2})
		}
		l1.lock.RUnlock()
	}
	m.root.lock.RUnlock()

	type workItem struct {
		level    *Level
		dir      string
		selector []string
	}

	n, errs := int32(0), int32(0)

	var wg sync.WaitGroup
	wg.Add(NumWorkers)
	work := make(chan workItem, NumWorkers*2)
	for worker := 0; worker < NumWorkers; worker++ {
		go func() {
			defer wg.Done()

			for workItem := range work {
				if err := workItem.level.toJSONCheckpoint(workItem.dir, from, to, m); err != nil {
					if err == ErrNoNewData {
						continue
					}

					log.Printf("error while checkpointing %#v: %s", workItem.selector, err.Error())
					atomic.AddInt32(&errs, 1)
				} else {
					atomic.AddInt32(&n, 1)
				}
			}
		}()
	}

	for i := 0; i < len(levels); i++ {
		dir := path.Join(dir, path.Join(selectors[i]...))
		work <- workItem{
			level:    levels[i],
			dir:      dir,
			selector: selectors[i],
		}
	}

	close(work)
	wg.Wait()

	if errs > 0 {
		return int(n), fmt.Errorf("%d errors happend while creating checkpoints (%d successes)", errs, n)
	}
	return int(n), nil
}

func (l *Level) toJSONCheckpointFile(from, to int64, m *MemoryStore) (*JSONCheckpointFile, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	retval := &JSONCheckpointFile{
		From:     from,
		To:       to,
		Metrics:  make(map[string]*JSONCheckpointMetrics),
		Children: make(map[string]*JSONCheckpointFile),
	}

	for metric, minfo := range m.metrics {
		b := l.metrics[minfo.Offset]
		if b == nil {
			continue
		}

		data := make([]types.Float, (to-from)/b.frequency+1)
		data, start, end, err := b.read(from, to, data)
		if err != nil {
			return nil, err
		}

		for i := int((end - start) / b.frequency); i < len(data); i++ {
			data[i] = types.NaN
		}

		retval.Metrics[metric] = &JSONCheckpointMetrics{
			Frequency: b.frequency,
			Start:     start,
			Data:      data,
		}
	}

	for name, child := range l.sublevels {
		val, err := child.toJSONCheckpointFile(from, to, m)
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

func (l *Level) toJSONCheckpoint(dir string, from, to int64, m *MemoryStore) error {
	cf, err := l.toJSONCheckpointFile(from, to, m)
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
func (m *MemoryStore) FromJSONCheckpoint(dir string, from int64) (int, error) {
	var wg sync.WaitGroup
	work := make(chan [2]string, NumWorkers)
	n, errs := int32(0), int32(0)

	wg.Add(NumWorkers)
	for worker := 0; worker < NumWorkers; worker++ {
		go func() {
			defer wg.Done()
			for host := range work {
				lvl := m.root.findLevelOrCreate(host[:], len(m.metrics))
				nn, err := lvl.fromJSONCheckpoint(filepath.Join(dir, host[0], host[1]), from, m)
				if err != nil {
					log.Fatalf("error while loading checkpoints: %s", err.Error())
					atomic.AddInt32(&errs, 1)
				}
				atomic.AddInt32(&n, int32(nn))
			}
		}()
	}

	i := 0
	clustersDir, err := os.ReadDir(dir)
	for _, clusterDir := range clustersDir {
		if !clusterDir.IsDir() {
			err = errors.New("expected only directories at first level of checkpoints/ directory")
			goto done
		}

		hostsDir, e := os.ReadDir(filepath.Join(dir, clusterDir.Name()))
		if e != nil {
			err = e
			goto done
		}

		for _, hostDir := range hostsDir {
			if !hostDir.IsDir() {
				err = errors.New("expected only directories at second level of checkpoints/ directory")
				goto done
			}

			i++
			if i%NumWorkers == 0 && i > 100 {
				// Forcing garbage collection runs here regulary during the loading of checkpoints
				// will decrease the total heap size after loading everything back to memory is done.
				// While loading data, the heap will grow fast, so the GC target size will double
				// almost always. By forcing GCs here, we can keep it growing more slowly so that
				// at the end, less memory is wasted.
				runtime.GC()
			}

			work <- [2]string{clusterDir.Name(), hostDir.Name()}
		}
	}
done:
	close(work)
	wg.Wait()

	if err != nil {
		return int(n), err
	}

	if errs > 0 {
		return int(n), fmt.Errorf("%d errors happend while creating checkpoints (%d successes)", errs, n)
	}
	return int(n), nil
}

func (l *Level) loadJSONCheckpointFile(cf *JSONCheckpointFile, m *MemoryStore) error {
	for name, metric := range cf.Metrics {
		n := len(metric.Data)
		c := &chunk{
			frequency:    metric.Frequency,
			start:        metric.Start,
			data:         metric.Data[0:n:n], // Space is wasted here :(
			prev:         nil,
			next:         nil,
			checkpointed: true,
		}

		mc, ok := m.metrics[name]
		if !ok {
			continue
			// return errors.New("Unkown metric: " + name)
		}

		prev := l.metrics[mc.Offset]
		if prev == nil {
			l.metrics[mc.Offset] = c
		} else {
			if prev.start > c.start {
				return errors.New("wooops")
			}

			c.prev = prev
			prev.next = c
		}
		l.metrics[mc.Offset] = c
	}

	if len(cf.Children) > 0 && l.sublevels == nil {
		l.sublevels = make(map[string]*Level)
	}

	for sel, childCf := range cf.Children {
		child, ok := l.sublevels[sel]
		if !ok {
			child = &Level{
				metrics:   make([]*chunk, len(m.metrics)),
				sublevels: nil,
			}
			l.sublevels[sel] = child
		}

		if err := child.loadJSONCheckpointFile(childCf, m); err != nil {
			return err
		}
	}

	return nil
}

func (l *Level) fromJSONCheckpoint(dir string, from int64, m *MemoryStore) (int, error) {
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
			child := &Level{
				metrics:   make([]*chunk, len(m.metrics)),
				sublevels: make(map[string]*Level),
			}

			files, err := child.fromJSONCheckpoint(path.Join(dir, e.Name()), from, m)
			filesLoaded += files
			if err != nil {
				return filesLoaded, err
			}

			l.sublevels[e.Name()] = child
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
		defer f.Close()

		br := bufio.NewReader(f)
		cf := &JSONCheckpointFile{}
		if err = json.NewDecoder(br).Decode(cf); err != nil {
			return filesLoaded, err
		}

		if cf.To != 0 && cf.To < from {
			continue
		}

		if err = l.loadJSONCheckpointFile(cf, m); err != nil {
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
		ts, err := strconv.ParseInt(
			strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".json"), ".data"), 10, 64)
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
func ArchiveCheckpoints(checkpointsDir, archiveDir string, from int64, deleteInstead bool) (int, error) {
	entries1, err := os.ReadDir(checkpointsDir)
	if err != nil {
		return 0, err
	}

	type workItem struct {
		cdir, adir    string
		cluster, host string
	}

	numWorkers := 3
	var wg sync.WaitGroup
	n, errs := int32(0), int32(0)
	work := make(chan workItem, numWorkers)

	wg.Add(numWorkers)
	for worker := 0; worker < numWorkers; worker++ {
		go func() {
			defer wg.Done()
			for workItem := range work {
				m, err := archiveCheckpoints(workItem.cdir, workItem.adir, from, deleteInstead)
				if err != nil {
					log.Printf("error while archiving %s/%s: %s", workItem.cluster, workItem.host, err.Error())
					atomic.AddInt32(&errs, 1)
				}
				atomic.AddInt32(&n, int32(m))
			}
		}()
	}

	for _, de1 := range entries1 {
		entries2, e := os.ReadDir(filepath.Join(checkpointsDir, de1.Name()))
		if e != nil {
			err = e
		}

		for _, de2 := range entries2 {
			cdir := filepath.Join(checkpointsDir, de1.Name(), de2.Name())
			adir := filepath.Join(archiveDir, de1.Name(), de2.Name())
			work <- workItem{
				adir: adir, cdir: cdir,
				cluster: de1.Name(), host: de2.Name(),
			}
		}
	}

	close(work)
	wg.Wait()

	if err != nil {
		return int(n), err
	}

	if errs > 0 {
		return int(n), fmt.Errorf("%d errors happend while archiving (%d successes)", errs, n)
	}
	return int(n), nil
}

// Helper function for `ArchiveCheckpoints`.
func archiveCheckpoints(dir string, archiveDir string, from int64, deleteInstead bool) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	files, err := findFiles(entries, from, false)
	if err != nil {
		return 0, err
	}

	if deleteInstead {
		n := 0
		for _, checkpoint := range files {
			filename := filepath.Join(dir, checkpoint)
			if err = os.Remove(filename); err != nil {
				return n, err
			}
			n += 1
		}
		return n, nil
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
		return 0, err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)
	defer bw.Flush()
	zw := zip.NewWriter(bw)
	defer zw.Close()

	n := 0
	for _, checkpoint := range files {
		filename := filepath.Join(dir, checkpoint)
		r, err := os.Open(filename)
		if err != nil {
			return n, err
		}
		defer r.Close()

		w, err := zw.Create(checkpoint)
		if err != nil {
			return n, err
		}

		if _, err = io.Copy(w, r); err != nil {
			return n, err
		}

		if err = os.Remove(filename); err != nil {
			return n, err
		}
		n += 1
	}

	return n, nil
}
