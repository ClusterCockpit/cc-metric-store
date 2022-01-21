package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/influxdata/line-protocol/v2/lineprotocol"
	"github.com/nats-io/nats.go"
)

// Go's JSON encoder for floats does not support NaN (https://github.com/golang/go/issues/3480).
// This program uses NaN as a signal for missing data.
// For the HTTP JSON API to be able to handle NaN values,
// we have to use our own type which implements encoding/json.Marshaler itself.
type Float float64

var NaN Float = Float(math.NaN())

func (f Float) IsNaN() bool {
	return math.IsNaN(float64(f))
}

func (f Float) MarshalJSON() ([]byte, error) {
	if math.IsNaN(float64(f)) {
		return []byte("null"), nil
	}

	return []byte(strconv.FormatFloat(float64(f), 'f', 2, 64)), nil
}

func (f *Float) UnmarshalJSON(input []byte) error {
	s := string(input)
	if s == "null" {
		*f = NaN
		return nil
	}

	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*f = Float(val)
	return nil
}

type Metric struct {
	Name  string
	Value Float
}

// Connect to a nats server and subscribe to "updates". This is a blocking
// function. handleLine will be called for each line recieved via nats.
// Send `true` through the done channel for gracefull termination.
func ReceiveNats(address string, handleLine func(dec *lineprotocol.Decoder) error, workers int, ctx context.Context) error {
	nc, err := nats.Connect(address)
	if err != nil {
		return err
	}
	defer nc.Close()

	var wg sync.WaitGroup
	var sub *nats.Subscription

	msgs := make(chan *nats.Msg, workers*2)

	if workers > 1 {
		wg.Add(workers)

		for i := 0; i < workers; i++ {
			go func() {
				for m := range msgs {
					dec := lineprotocol.NewDecoderWithBytes(m.Data)
					if err := handleLine(dec); err != nil {
						log.Printf("error: %s\n", err.Error())
					}
				}

				wg.Done()
			}()
		}

		sub, err = nc.Subscribe("updates", func(m *nats.Msg) {
			msgs <- m
		})
	} else {
		sub, err = nc.Subscribe("updates", func(m *nats.Msg) {
			dec := lineprotocol.NewDecoderWithBytes(m.Data)
			if err := handleLine(dec); err != nil {
				log.Printf("error: %s\n", err.Error())
			}
		})
	}

	if err != nil {
		return err
	}

	log.Printf("NATS subscription to 'updates' on '%s' established\n", address)

	<-ctx.Done()
	err = sub.Unsubscribe()
	close(msgs)
	wg.Wait()

	if err != nil {
		return err
	}

	nc.Close()
	log.Println("NATS connection closed")
	return nil
}

func decodeLine(dec *lineprotocol.Decoder) error {
	// Reduce allocations in loop:
	t := time.Now()
	metrics := make([]Metric, 0, 10)
	selector := make([]string, 0, 4)
	typeBuf, subTypeBuf := make([]byte, 0, 20), make([]byte, 0)

	// Optimize for the case where all lines in a "batch" are about the same
	// cluster and host. By using `WriteToLevel` (level = host), we do not need
	// to take the root- and cluster-level lock as often.
	var hostLevel *level = nil
	var prevCluster, prevHost string = "", ""

	for dec.Next() {
		rawmeasurement, err := dec.Measurement()
		if err != nil {
			return err
		}

		var cluster, host string
		var typeName, typeId, subType, subTypeId []byte
		for {
			key, val, err := dec.NextTag()
			if err != nil {
				return err
			}
			if key == nil {
				break
			}

			// The go compiler optimizes string([]byte{...}) == "...":
			switch string(key) {
			case "cluster":
				if string(val) == prevCluster {
					cluster = prevCluster
				} else {
					cluster = string(val)
				}
			case "hostname":
				if string(val) == prevHost {
					host = prevHost
				} else {
					host = string(val)
				}
			case "type":
				typeName = val
			case "type-id":
				typeId = val
			case "subtype":
				subType = val
			case "stype-id":
				subTypeId = val
			default:
				// Ignore unkown tags (cc-metric-collector might send us a unit for example that we do not need)
				// return fmt.Errorf("unkown tag: '%s' (value: '%s')", string(key), string(val))
			}
		}

		if hostLevel == nil || prevCluster != cluster || prevHost != host {
			prevCluster = cluster
			prevHost = host
			selector = selector[:2]
			selector[0] = cluster
			selector[1] = host
			hostLevel = memoryStore.root.findLevelOrCreate(selector, len(memoryStore.metrics))
		}

		selector = selector[:0]
		if len(typeId) > 0 {
			typeBuf = typeBuf[:0]
			typeBuf = append(typeBuf, typeName...)
			typeBuf = append(typeBuf, typeId...)
			selector = append(selector, string(typeBuf)) // <- Allocation :(
			if len(subTypeId) > 0 {
				subTypeBuf = subTypeBuf[:0]
				subTypeBuf = append(subTypeBuf, subType...)
				subTypeBuf = append(subTypeBuf, subTypeId...)
				selector = append(selector, string(subTypeBuf))
			}
		}

		metrics = metrics[:0]
		// A more dense lp format if supported if the measurement is 'data'.
		// In that case, the field keys are used as metric names.
		if string(rawmeasurement) == "data" {
			for {
				key, val, err := dec.NextField()
				if err != nil {
					return err
				}

				if key == nil {
					break
				}

				var value Float
				if val.Kind() == lineprotocol.Float {
					value = Float(val.FloatV())
				} else if val.Kind() == lineprotocol.Int {
					value = Float(val.IntV())
				} else {
					return fmt.Errorf("unsupported value type in message: %s", val.Kind().String())
				}

				metrics = append(metrics, Metric{
					Name:  string(key), // <- Allocation :(
					Value: value,
				})
			}
		} else {
			measurement := string(rawmeasurement) // <- Allocation :(
			var value Float
			for {
				key, val, err := dec.NextField()
				if err != nil {
					return err
				}

				if key == nil {
					break
				}

				if string(key) != "value" {
					return fmt.Errorf("unkown field: '%s' (value: %#v)", string(key), val)
				}

				if val.Kind() == lineprotocol.Float {
					value = Float(val.FloatV())
				} else if val.Kind() == lineprotocol.Int {
					value = Float(val.IntV())
				} else {
					return fmt.Errorf("unsupported value type in message: %s", val.Kind().String())
				}
			}

			metrics = append(metrics, Metric{
				Name:  measurement,
				Value: value,
			})
		}

		t, err = dec.Time(lineprotocol.Second, t)
		if err != nil {
			return err
		}

		// log.Printf("write: %s (%v) -> %v\n", string(measurement), selector, value)
		if err := memoryStore.WriteToLevel(hostLevel, selector, t.Unix(), metrics); err != nil {
			return err
		}
	}
	return nil
}
