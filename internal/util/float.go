package util

import (
	"math"
	"strconv"
)

// Go's JSON encoder for floats does not support NaN (https://github.com/golang/go/issues/3480).
// This program uses NaN as a signal for missing data.
// For the HTTP JSON API to be able to handle NaN values,
// we have to use our own type which implements encoding/json.Marshaler itself.
type Float float64

var (
	NaN         Float  = Float(math.NaN())
	nullAsBytes []byte = []byte("null")
)

func (f Float) IsNaN() bool {
	return math.IsNaN(float64(f))
}

func (f Float) MarshalJSON() ([]byte, error) {
	if math.IsNaN(float64(f)) {
		return nullAsBytes, nil
	}

	return strconv.AppendFloat(make([]byte, 0, 10), float64(f), 'f', 3, 64), nil
}

func (f *Float) UnmarshalJSON(input []byte) error {
	if string(input) == "null" {
		*f = NaN
		return nil
	}

	val, err := strconv.ParseFloat(string(input), 64)
	if err != nil {
		return err
	}
	*f = Float(val)
	return nil
}

// Same as `[]Float`, but can be marshaled to JSON with less allocations.
type FloatArray []Float

func (fa FloatArray) MarshalJSON() ([]byte, error) {
	buf := make([]byte, 0, 2+len(fa)*8)
	buf = append(buf, '[')
	for i := 0; i < len(fa); i++ {
		if i != 0 {
			buf = append(buf, ',')
		}

		if fa[i].IsNaN() {
			buf = append(buf, `null`...)
		} else {
			buf = strconv.AppendFloat(buf, float64(fa[i]), 'f', 3, 64)
		}
	}
	buf = append(buf, ']')
	return buf, nil
}
