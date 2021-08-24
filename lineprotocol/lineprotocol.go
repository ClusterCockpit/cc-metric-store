package lineprotocol

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

// Go's JSON encoder for floats does not support NaN (https://github.com/golang/go/issues/3480).
// This program uses NaN as a signal for missing data.
// For the HTTP JSON API to be able to handle NaN values,
// we have to use our own type which implements encoding/json.Marshaler itself.
type Float float64

func (f Float) MarshalJSON() ([]byte, error) {
	if math.IsNaN(float64(f)) {
		return []byte("null"), nil
	}

	return []byte(strconv.FormatFloat(float64(f), 'f', -1, 64)), nil
}

type Metric struct {
	Name  string
	Value Float
}

// measurement: node or cpu
// tags:        host, cluster, cpu (cpu only if measurement is cpu)
// fields:      metrics...
// t:           timestamp (accuracy: seconds)
type Line struct {
	Measurement string
	Tags        map[string]string
	Fields      []Metric
	Ts          time.Time
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
	line.Measurement = tagsAndMeasurement[0]
	line.Tags = map[string]string{}
	for i := 1; i < len(tagsAndMeasurement); i++ {
		pair := strings.Split(tagsAndMeasurement[i], "=")
		if len(pair) != 2 {
			return nil, errors.New("line format error")
		}
		line.Tags[pair[0]] = pair[1]
	}

	rawfields := strings.Split(parts[1], ",")
	line.Fields = []Metric{}
	for i := 0; i < len(rawfields); i++ {
		pair := strings.Split(rawfields[i], "=")
		if len(pair) != 2 {
			return nil, errors.New("line format error")
		}
		field, err := strconv.ParseFloat(pair[1], 64)
		if err != nil {
			return nil, err
		}

		line.Fields = append(line.Fields, Metric{
			Name:  pair[0],
			Value: Float(field),
		})
	}

	unixTimestamp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, err
	}

	line.Ts = time.Unix(unixTimestamp, 0)
	return line, nil
}
