package memorystore

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"time"

	"github.com/ClusterCockpit/cc-metric-store/internal/avro"
	"github.com/ClusterCockpit/cc-metric-store/internal/config"
	"github.com/ClusterCockpit/cc-metric-store/internal/util"
)

// Whenever changed, update MarshalJSON as well!
type CheckpointMetrics struct {
	Data      []util.Float `json:"data"`
	Frequency int64        `json:"frequency"`
	Start     int64        `json:"start"`
}

type CheckpointFile struct {
	Metrics  map[string]*CheckpointMetrics `json:"metrics"`
	Children map[string]*CheckpointFile    `json:"children"`
	From     int64                         `json:"from"`
	To       int64                         `json:"to"`
}

var lastCheckpoint time.Time

func Checkpointing(wg *sync.WaitGroup, ctx context.Context) {
	lastCheckpoint = time.Now()

	if config.Keys.Checkpoints.FileFormat == "json" {
		ms := GetMemoryStore()

		go func() {
			defer wg.Done()
			d, err := time.ParseDuration(config.Keys.Checkpoints.Interval)
			if err != nil {
				log.Fatal(err)
			}
			if d <= 0 {
				return
			}

			ticks := func() <-chan time.Time {
				if d <= 0 {
					return nil
				}
				return time.NewTicker(d).C
			}()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticks:
					log.Printf("start checkpointing (starting at %s)...\n", lastCheckpoint.Format(time.RFC3339))
					now := time.Now()
					n, err := ms.ToCheckpoint(config.Keys.Checkpoints.RootDir,
						lastCheckpoint.Unix(), now.Unix())
					if err != nil {
						log.Printf("checkpointing failed: %s\n", err.Error())
					} else {
						log.Printf("done: %d checkpoint files created\n", n)
						lastCheckpoint = now
					}
				}
			}
		}()
	} else {
		go func() {
			defer wg.Done()
			d, _ := time.ParseDuration("1m")

			d_cp, err := time.ParseDuration(config.Keys.Checkpoints.Interval)
			if err != nil {
				log.Fatal(err)
			}
			if d_cp <= 0 {
				return
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(avro.CheckpointBufferMinutes) * time.Minute):
				// This is the first tick untill we collect the data for given minutes.
				avro.GetAvroStore().ToCheckpoint(config.Keys.Checkpoints.RootDir)
			}

			ticks := func() <-chan time.Time {
				if d <= 0 {
					return nil
				}
				return time.NewTicker(d).C
			}()

			ticks_cp := func() <-chan time.Time {
				if d_cp <= 0 {
					return nil
				}
				return time.NewTicker(d_cp).C
			}()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticks_cp:
					lastCheckpoint = time.Now()
				case <-ticks:
					// Regular ticks of 1 minute to write data.
					avro.GetAvroStore().ToCheckpoint(config.Keys.Checkpoints.RootDir)
				}
			}
		}()
	}
}

// As `Float` implements a custom MarshalJSON() function,
// serializing an array of such types has more overhead
// than one would assume (because of extra allocations, interfaces and so on).
func (cm *CheckpointMetrics) MarshalJSON() ([]byte, error) {
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

// Metrics stored at the lowest 2 levels are not stored away (root and cluster)!
// On a per-host basis a new JSON file is created. I have no idea if this will scale.
// The good thing: Only a host at a time is locked, so this function can run
// in parallel to writes/reads.
func (m *MemoryStore) ToCheckpoint(dir string, from, to int64) (int, error) {
	levels := make([]*Level, 0)
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
				if err := workItem.level.toCheckpoint(workItem.dir, from, to, m); err != nil {
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

func (l *Level) toCheckpointFile(from, to int64, m *MemoryStore) (*CheckpointFile, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()

	retval := &CheckpointFile{
		From:     from,
		To:       to,
		Metrics:  make(map[string]*CheckpointMetrics),
		Children: make(map[string]*CheckpointFile),
	}

	for metric, minfo := range m.Metrics {
		b := l.metrics[minfo.Offset]
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

		data := make([]util.Float, (to-from)/b.frequency+1)
		data, start, end, err := b.read(from, to, data)
		if err != nil {
			return nil, err
		}

		for i := int((end - start) / b.frequency); i < len(data); i++ {
			data[i] = util.NaN
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

func (l *Level) toCheckpoint(dir string, from, to int64, m *MemoryStore) error {
	cf, err := l.toCheckpointFile(from, to, m)
	if err != nil {
		return err
	}

	if cf == nil {
		return ErrNoNewData
	}

	filepath := path.Join(dir, fmt.Sprintf("%d.json", from))
	f, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0o755)
		if err == nil {
			f, err = os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY, 0o644)
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
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// The directory does not exist, so create it using os.MkdirAll()
		err := os.MkdirAll(dir, 0755) // 0755 sets the permissions for the directory
		if err != nil {
			log.Fatalf("Error creating directory: %#v\n", err)
		}
		fmt.Printf("%#v Directory created successfully.\n", dir)
	}

	var wg sync.WaitGroup
	work := make(chan [2]string, NumWorkers)
	n, errs := int32(0), int32(0)

	wg.Add(NumWorkers)
	for worker := 0; worker < NumWorkers; worker++ {
		go func() {
			defer wg.Done()
			for host := range work {
				lvl := m.root.findLevelOrCreate(host[:], len(m.Metrics))
				nn, err := lvl.fromCheckpoint(filepath.Join(dir, host[0], host[1]), from, m)
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

func (l *Level) loadFile(cf *CheckpointFile, m *MemoryStore) error {
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

		minfo, ok := m.Metrics[name]
		if !ok {
			continue
			// return errors.New("Unkown metric: " + name)
		}

		prev := l.metrics[minfo.Offset]
		if prev == nil {
			l.metrics[minfo.Offset] = b
		} else {
			if prev.start > b.start {
				return errors.New("wooops")
			}

			b.prev = prev
			prev.next = b
		}
		l.metrics[minfo.Offset] = b
	}

	if len(cf.Children) > 0 && l.children == nil {
		l.children = make(map[string]*Level)
	}

	for sel, childCf := range cf.Children {
		child, ok := l.children[sel]
		if !ok {
			child = &Level{
				metrics:  make([]*buffer, len(m.Metrics)),
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

func (l *Level) fromCheckpoint(dir string, from int64, m *MemoryStore) (int, error) {
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
				metrics:  make([]*buffer, len(m.Metrics)),
				children: make(map[string]*Level),
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
		defer f.Close()

		br := bufio.NewReader(f)
		cf := &CheckpointFile{}
		if err = json.NewDecoder(br).Decode(cf); err != nil {
			return filesLoaded, err
		}

		if cf.To != 0 && cf.To < from {
			continue
		}

		if err = l.loadFile(cf, m); err != nil {
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

		if findMoreRecentFiles && t <= ts1 {
			filenames = append(filenames, e.Name())
		}
		if i == len(direntries)-1 {
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
