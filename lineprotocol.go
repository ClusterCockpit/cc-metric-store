package main

import (
	"bufio"
	"context"
	"log"
	"math"
	"net"
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

// Listen for connections sending metric data in the line protocol format.
//
// This is a blocking function, send `true` through the channel argument to shut down the server.
// `handleLine` will be called from different go routines for different connections.
//
func ReceiveTCP(address string, handleLine func(dec *lineprotocol.Decoder), done chan bool) error {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	go func() {
		for {
			stop := <-done
			if stop {
				err := ln.Close()
				if err != nil {
					log.Printf("closing listener failed: %s\n", err.Error())
				}
				return
			}
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}

		go func() {
			reader := bufio.NewReader(conn)
			dec := lineprotocol.NewDecoder(reader)
			handleLine(dec)
		}()
	}
}

// Connect to a nats server and subscribe to "updates". This is a blocking
// function. handleLine will be called for each line recieved via nats.
// Send `true` through the done channel for gracefull termination.
func ReceiveNats(address string, handleLine func(dec *lineprotocol.Decoder), workers int, ctx context.Context) error {
	nc, err := nats.Connect(nats.DefaultURL)
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
					handleLine(dec)
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
			handleLine(dec)
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
