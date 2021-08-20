package lineprotocol

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

type Metric struct {
	Name  string
	Value float64
}

// measurement: node or cpu
// tags:        host, cluster, cpu (cpu only if measurement is cpu)
// fields:      metrics...
// t:           timestamp (accuracy: seconds)
type Line struct {
	measurement string
	tags        map[string]string
	fields      []Metric
	t           time.Time
}

// Parse a single line as string.
//
// There is performance to be gained by implementing a parser
// that directly reads from a bufio.Scanner.
func Parse(rawline string) (*Line, error) {
	line := &Line{}
	parts := strings.Fields(rawline)
	if len(parts) != 3 {
		return nil, errors.New("line format error")
	}

	tagsAndMeasurement := strings.Split(parts[0], ",")
	line.measurement = tagsAndMeasurement[0]
	line.tags = map[string]string{}
	for i := 1; i < len(tagsAndMeasurement); i++ {
		pair := strings.Split(tagsAndMeasurement[i], "=")
		if len(pair) != 2 {
			return nil, errors.New("line format error")
		}
		line.tags[pair[0]] = pair[1]
	}

	rawfields := strings.Split(parts[1], ",")
	line.fields = []Metric{}
	for i := 0; i < len(rawfields); i++ {
		pair := strings.Split(rawfields[i], "=")
		if len(pair) != 2 {
			return nil, errors.New("line format error")
		}
		field, err := strconv.ParseFloat(pair[1], 64)
		if err != nil {
			return nil, err
		}

		line.fields = append(line.fields, Metric{
			Name:  pair[0],
			Value: field,
		})
	}

	unixTimestamp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, err
	}

	line.t = time.Unix(unixTimestamp, 0)
	return line, nil
}
