package main

import (
	"context"
	"log"
	"math"
	"strconv"
	"sync"

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
