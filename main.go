package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"

	"github.com/BurntSushi/toml"
)

var wg sync.WaitGroup

func main() {
	ports, err := readPorts()
	if err != nil {
		log.Fatalln(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan os.Signal, 1)
	signal.Notify(doneCh, os.Interrupt)
	go func() {
		<-doneCh

		cancel()
	}()

	// // start tcp listener on each port
	// for _, port := range ports {
	// 	if err := echoServer(ctx, port); err != nil {
	// 		log.Fatal(err)
	// 	}
	// }

	go echoServer(ctx, 8080, noop)
	go echoServer(ctx, 8081, bytes.ToUpper)

	// start http server
	go func() {
		http.HandleFunc("/", httpHandler(ports))

		log.Println("http listening on", 80)

		if err := http.ListenAndServe(fmt.Sprintf(":%d", 80), nil); err != nil {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()

	wg.Wait()

	log.Println("goodbye")
}

func echoServer(ctx context.Context, port int, transform transformFn) {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("tcp listening on", ":"+strconv.Itoa(port))
	wg.Add(1)
	defer wg.Done()
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()

			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("[%d] listen error %s\n", port, err)
					continue
				}
			}
			log.Printf("[%d] %s connected\n", port, conn.RemoteAddr())

			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()

				n, err := copy(conn, transform)

				if err != nil {
					log.Printf("[%d] %s copy error %s\n", port, conn.RemoteAddr(), err)
				}
				log.Printf("[%d] %s echoed %d bytes\n", port, conn.RemoteAddr(), n)
			}()
		}
	}()

	<-ctx.Done()
}

type transformFn func(data []byte) []byte

func copy(conn net.Conn, transform transformFn) (int64, error) {
	var n int64
	buf := make([]byte, 1024)
	for {
		size, err := conn.Read(buf[:])
		n += int64(size)

		if err != nil {
			if errors.Is(err, io.EOF) {
				return n, nil
			}
			return n, err
		}

		if _, err = conn.Write(transform(buf[:size])); err != nil {
			return n, err
		}
	}
}

func noop(data []byte) []byte {
	return data
}

func httpHandler(ports []int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello!")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "I'm a raw TCP echo service. Whatever you send to me will be sent right back to you.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "I'm listening on a bunch of ports. Even numbered ports return data as is. Odd numbered ports SHOUT the data back at you.")

		fmt.Fprintln(w)
		fmt.Fprintln(w, "Give these a try:")

		for _, port := range ports {
			fmt.Fprintf(w, " - tcp://%s.fly.dev:%d\n", "tcp-echo", port)
		}
	}
}

type cfg struct {
	Services []struct {
		Ports []struct {
			Handlers []string
			Port     int
		}
	}
}

func readPorts() ([]int, error) {
	var conf cfg
	_, err := toml.DecodeFile("./fly.toml", &conf)
	if err != nil {
		return nil, err
	}

	var ports []int
	for _, svc := range conf.Services {
		for _, port := range svc.Ports {
			if len(port.Handlers) > 0 {
				continue
			}
			ports = append(ports, port.Port)

		}
	}

	sort.Ints(ports)

	return ports, nil
}
