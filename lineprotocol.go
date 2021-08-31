package main

import (
	"bufio"
	"errors"
	"io"
	"log"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

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

	return []byte(strconv.FormatFloat(float64(f), 'f', -1, 64)), nil
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

// Listen for connections sending metric data in the line protocol format.
//
// This is a blocking function, send `true` through the channel argument to shut down the server.
// `handleLine` will be called from different go routines for different connections.
//
func ReceiveTCP(address string, handleLine func(line *Line), done chan bool) error {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	handleConnection := func(conn net.Conn, handleLine func(line *Line)) {
		reader := bufio.NewReader(conn)
		for {
			rawline, err := reader.ReadString('\n')
			if err == io.EOF {
				return
			}

			if err != nil {
				log.Printf("reading from connection failed: %s\n", err.Error())
				return
			}

			line, err := Parse(rawline)
			if err != nil {
				log.Printf("parsing line failed: %s\n", err.Error())
				return
			}

			handleLine(line)
		}
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

		go handleConnection(conn, handleLine)
	}
}

// Connect to a nats server and subscribe to "updates". This is a blocking
// function. handleLine will be called for each line recieved via nats.
// Send `true` through the done channel for gracefull termination.
func ReceiveNats(address string, handleLine func(line *Line), done chan bool) error {
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		return err
	}
	defer nc.Close()

	// Subscribe
	if _, err := nc.Subscribe("updates", func(m *nats.Msg) {
		line, err := Parse(string(m.Data))
		if err != nil {
			log.Printf("parsing line failed: %s\n", err.Error())
			return
		}

		handleLine(line)
	}); err != nil {
		return err
	}

	log.Printf("NATS subscription to 'updates' on '%s' established\n", address)
	for {
		_ = <-done
		log.Println("NATS connection closed")
		return nil
	}
}
