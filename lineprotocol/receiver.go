package lineprotocol

import (
	"bufio"
	"io"
	"log"
	"net"

	nats "github.com/nats-io/nats.go"
)

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

	for {
		stop := <-done
		if stop {
			return nil
		}
	}
}
