package avro

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClusterCockpit/cc-metric-store/internal/util"
	"github.com/linkedin/goavro/v2"
)

var NumWorkers int = 4

var ErrNoNewData error = errors.New("no data in the pool")

func (as *AvroStore) ToCheckpoint(dir string) (int, error) {
	levels := make([]*AvroLevel, 0)
	selectors := make([][]string, 0)
	as.root.lock.RLock()
	// Cluster
	for sel1, l1 := range as.root.children {
		l1.lock.RLock()
		// Node
		for sel2, l2 := range l1.children {
			l2.lock.RLock()
			// Frequency
			for sel3, l3 := range l1.children {
				levels = append(levels, l3)
				selectors = append(selectors, []string{sel1, sel2, sel3})
			}
			l2.lock.RUnlock()
		}
		l1.lock.RUnlock()
	}
	as.root.lock.RUnlock()

	type workItem struct {
		level    *AvroLevel
		dir      string
		selector []string
	}

	n, errs := int32(0), int32(0)

	var wg sync.WaitGroup
	wg.Add(NumWorkers)
	work := make(chan workItem, NumWorkers*2)
	for range NumWorkers {
		go func() {
			defer wg.Done()

			for workItem := range work {
				var from int64 = getTimestamp(workItem.dir)

				if err := workItem.level.toCheckpoint(workItem.dir, from); err != nil {
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

	for i := range len(levels) {
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
		return int(n), fmt.Errorf("%d errors happend while creating avro checkpoints (%d successes)", errs, n)
	}
	return int(n), nil
}

// getTimestamp returns the timestamp from the directory name
func getTimestamp(dir string) int64 {
	// Extract the timestamp from the directory name
	// The existing avro file will be in epoch timestamp format
	// iterate over all the files in the directory and find the maximum timestamp
	// and return it
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var maxTs int64 = 0

	if len(files) == 0 {
		return 0
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		if len(name) < 5 {
			continue
		}
		ts, err := strconv.ParseInt(name[:len(name)-5], 10, 64)
		if err != nil {
			continue
		}
		if ts > maxTs {
			maxTs = ts
		}
	}

	return maxTs
}

func (l *AvroLevel) toCheckpoint(dir string, from int64) error {

	fmt.Printf("Checkpointing directory: %s\n", dir)

	// find smallest overall timestamp in l.data map and delete it from l.data
	var minTs int64 = int64(1<<63 - 1)
	for ts := range l.data {
		if ts < minTs {
			minTs = ts
		}
	}

	if from == 0 {
		from = minTs
	}

	var schema string
	var codec *goavro.Codec
	record_list := make([]map[string]interface{}, 0)

	var f *os.File

	filePath := path.Join(dir, fmt.Sprintf("%d.avro", from))

	if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		err = os.MkdirAll(dir, 0o755)
		if err == nil {
			f, err = os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return fmt.Errorf("failed to create new avro file: %v", err)
			}
		}
	} else {
		f, err = os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open existing avro file: %v", err)
		}
		reader, err := goavro.NewOCFReader(f)
		if err != nil {
			return fmt.Errorf("failed to create OCF reader: %v", err)
		}
		schema = reader.Codec().Schema()

		f.Close()

		f, err = os.OpenFile(filePath, os.O_APPEND|os.O_RDWR, 0o644)
		if err != nil {
			log.Fatalf("Failed to create file: %v", err)
		}
	}
	defer f.Close()

	time_ref := time.Now().Add(time.Duration(-CheckpointBufferMinutes) * time.Minute).Unix()

	for ts := range l.data {
		if ts < time_ref {
			schema_gen, err := generateSchema(l.data[ts])
			if err != nil {
				return err
			}

			flag, schema, err := compareSchema(schema, schema_gen)
			if err != nil {
				log.Fatalf("Failed to compare read and generated schema: %v", err)
			}
			if flag {
				codec, err = goavro.NewCodec(schema)
				if err != nil {
					log.Fatalf("Failed to create codec after merged schema: %v", err)
				}

				f.Close()

				f, err = os.Open(filePath)
				if err != nil {
					log.Fatalf("Failed to open Avro file: %v", err)
				}

				ocfReader, err := goavro.NewOCFReader(f)
				if err != nil {
					log.Fatalf("Failed to create OCF reader: %v", err)
				}

				for ocfReader.Scan() {
					record, err := ocfReader.Read()
					if err != nil {
						log.Fatalf("Failed to read record: %v", err)
					}

					record_list = append(record_list, record.(map[string]interface{}))
				}

				f.Close()

				err = os.Remove(filePath)
				if err != nil {
					log.Fatalf("Failed to delete file: %v", err)
				}

				f, err = os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0o644)
				if err != nil {
					log.Fatalf("Failed to create file after deleting : %v", err)
				}
			}

			record_list = append(record_list, generateRecord(l.data[ts]))
			delete(l.data, minTs)
		}
	}

	if len(record_list) == 0 {
		return ErrNoNewData
	}

	writer, err := goavro.NewOCFWriter(goavro.OCFConfig{
		W:      f,
		Codec:  codec,
		Schema: schema,
	})
	if err != nil {
		log.Fatalf("Failed to create OCF writer: %v", err)
	}

	// Append the new record
	if err := writer.Append(record_list); err != nil {
		log.Fatalf("Failed to append record: %v", err)
	}

	return nil
}

func compareSchema(schemaRead, schemaGen string) (bool, string, error) {
	var genSchema, readSchema AvroSchema

	if schemaRead == "" {
		return false, schemaGen, nil
	}

	// Unmarshal the schema strings into AvroSchema structs
	if err := json.Unmarshal([]byte(schemaGen), &genSchema); err != nil {
		return false, "", fmt.Errorf("failed to parse generated schema: %v", err)
	}
	if err := json.Unmarshal([]byte(schemaRead), &readSchema); err != nil {
		return false, "", fmt.Errorf("failed to parse read schema: %v", err)
	}

	sort.Slice(genSchema.Fields, func(i, j int) bool {
		return genSchema.Fields[i].Name < genSchema.Fields[j].Name
	})

	sort.Slice(readSchema.Fields, func(i, j int) bool {
		return readSchema.Fields[i].Name < readSchema.Fields[j].Name
	})

	// Check if schemas are identical
	schemasEqual := true
	if len(genSchema.Fields) <= len(readSchema.Fields) {

		for i := range genSchema.Fields {
			if genSchema.Fields[i].Name != readSchema.Fields[i].Name {
				schemasEqual = false
				break
			}
		}

		// If schemas are identical, return the read schema
		if schemasEqual {
			return false, schemaRead, nil
		}
	}

	// Create a map to hold unique fields from both schemas
	fieldMap := make(map[string]AvroField)

	// Add fields from the read schema
	for _, field := range readSchema.Fields {
		fieldMap[field.Name] = field
	}

	// Add or update fields from the generated schema
	for _, field := range genSchema.Fields {
		fieldMap[field.Name] = field
	}

	// Create a union schema by collecting fields from the map
	var mergedFields []AvroField
	for _, field := range fieldMap {
		mergedFields = append(mergedFields, field)
	}

	// Sort fields by name for consistency
	sort.Slice(mergedFields, func(i, j int) bool {
		return mergedFields[i].Name < mergedFields[j].Name
	})

	// Create the merged schema
	mergedSchema := AvroSchema{
		Type:   "record",
		Name:   genSchema.Name,
		Fields: mergedFields,
	}

	// Check if schemas are identical
	schemasEqual = len(mergedSchema.Fields) == len(readSchema.Fields)
	if schemasEqual {
		for i := range mergedSchema.Fields {
			if mergedSchema.Fields[i].Name != readSchema.Fields[i].Name {
				schemasEqual = false
				break
			}
		}

		if schemasEqual {
			return false, schemaRead, nil
		}
	}

	// Marshal the merged schema back to JSON
	mergedSchemaJson, err := json.Marshal(mergedSchema)
	if err != nil {
		return false, "", fmt.Errorf("failed to marshal merged schema: %v", err)
	}

	fmt.Printf("Merged Schema: %s\n", string(mergedSchemaJson))
	fmt.Printf("Read Schema: %s\n", schemaRead)

	return true, string(mergedSchemaJson), nil
}

func generateSchema(data map[string]util.Float) (string, error) {

	// Define the Avro schema structure
	schema := map[string]interface{}{
		"type":   "record",
		"name":   "DataRecord",
		"fields": []map[string]interface{}{},
	}

	fieldTracker := make(map[string]struct{})

	for key := range data {
		if _, exists := fieldTracker[key]; !exists {
			field := map[string]interface{}{
				"name":    key,
				"type":    "double", // Allows null or float
				"default": 0.0,
			}
			schema["fields"] = append(schema["fields"].([]map[string]interface{}), field)
			fieldTracker[key] = struct{}{}
		}
	}

	schemaString, err := json.Marshal(schema)
	if err != nil {
		return "", fmt.Errorf("failed to marshal schema: %v", err)
	}

	return string(schemaString), nil
}
func generateRecord(data map[string]util.Float) map[string]interface{} {

	record := make(map[string]interface{})

	// Iterate through each map in data
	for key, value := range data {
		// Set the value in the record
		record[key] = value
	}

	return record
}
