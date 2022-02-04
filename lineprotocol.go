package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/influxdata/line-protocol/v2/lineprotocol"
	"github.com/nats-io/nats.go"
)

type Metric struct {
	Name  string
	minfo metricInfo
	Value Float
}

// Connect to a nats server and subscribe to "updates". This is a blocking
// function. handleLine will be called for each line recieved via nats.
// Send `true` through the done channel for gracefull termination.
func ReceiveNats(conf *NatsConfig, handleLine func(dec *lineprotocol.Decoder) error, workers int, ctx context.Context) error {
	var opts []nats.Option
	if conf.Username != "" && conf.Password != "" {
		opts = append(opts, nats.UserInfo(conf.Username, conf.Password))
	}

	nc, err := nats.Connect(conf.Address, opts...)
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

		sub, err = nc.Subscribe(conf.SubscribeTo, func(m *nats.Msg) {
			msgs <- m
		})
	} else {
		sub, err = nc.Subscribe(conf.SubscribeTo, func(m *nats.Msg) {
			dec := lineprotocol.NewDecoderWithBytes(m.Data)
			if err := handleLine(dec); err != nil {
				log.Printf("error: %s\n", err.Error())
			}
		})
	}

	if err != nil {
		return err
	}

	log.Printf("NATS subscription to '%s' on '%s' established\n", conf.SubscribeTo, conf.Address)

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

// Place `prefix` in front of `buf` but if possible,
// do that inplace in `buf`.
func reorder(buf, prefix []byte) []byte {
	n := len(prefix)
	m := len(buf)
	if cap(buf) < m+n {
		return append(prefix[:n:n], buf...)
	} else {
		buf = buf[:n+m]
		for i := m - 1; i >= 0; i-- {
			buf[i+n] = buf[i]
		}
		for i := 0; i < n; i++ {
			buf[i] = prefix[i]
		}
		return buf
	}
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
	var lvl *level = nil
	var prevCluster, prevHost string = "", ""

	for dec.Next() {
		metrics = metrics[:0]
		rawmeasurement, err := dec.Measurement()
		if err != nil {
			return err
		}

		// A more dense lp format if supported if the measurement is 'data'.
		// In that case, the field keys are used as metric names.
		if string(rawmeasurement) != "data" {
			minfo, ok := memoryStore.metrics[string(rawmeasurement)]
			if !ok {
				continue
			}

			metrics = append(metrics, Metric{
				minfo: minfo,
			})
		}

		typeBuf, subTypeBuf := typeBuf[:0], subTypeBuf[:0]
		var cluster, host string
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
					lvl = nil
				}
			case "hostname", "host":
				if string(val) == prevHost {
					host = prevHost
				} else {
					host = string(val)
					lvl = nil
				}
			case "type":
				if string(val) == "node" {
					break
				}

				if len(typeBuf) == 0 {
					typeBuf = append(typeBuf, val...)
				} else {
					typeBuf = reorder(typeBuf, val)
				}
			case "type-id":
				typeBuf = append(typeBuf, val...)
			case "subtype":
				if len(subTypeBuf) == 0 {
					subTypeBuf = append(subTypeBuf, val...)
				} else {
					subTypeBuf = reorder(typeBuf, val)
				}
			case "stype-id":
				subTypeBuf = append(subTypeBuf, val...)
			default:
				// Ignore unkown tags (cc-metric-collector might send us a unit for example that we do not need)
				// return fmt.Errorf("unkown tag: '%s' (value: '%s')", string(key), string(val))
			}
		}

		if lvl == nil {
			selector = selector[:2]
			selector[0], selector[1] = cluster, host
			lvl = memoryStore.GetLevel(selector)
			prevCluster, prevHost = cluster, host
		}

		selector = selector[:0]
		if len(typeBuf) > 0 {
			selector = append(selector, string(typeBuf)) // <- Allocation :(
			if len(subTypeBuf) > 0 {
				selector = append(selector, string(subTypeBuf))
			}
		}

		if len(metrics) == 0 {
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

				minfo, ok := memoryStore.metrics[string(key)]
				if !ok {
					continue
				}

				metrics = append(metrics, Metric{
					minfo: minfo,
					Value: value,
				})
			}
		} else {
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

			metrics[0].Value = value
		}

		t, err = dec.Time(lineprotocol.Second, t)
		if err != nil {
			return err
		}

		// log.Printf("write: %s (%v) -> %v\n", string(measurement), selector, value)
		if err := memoryStore.WriteToLevel(lvl, selector, t.Unix(), metrics); err != nil {
			return err
		}
	}
	return nil
}
