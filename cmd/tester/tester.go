package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	signal.Ignore(syscall.SIGPIPE)
	for i := range 100 {
		if _, err := fmt.Fprintf(os.Stdout, "LINE %d\n", i+1); err != nil {
			fmt.Fprintf(os.Stderr, "err: %+v\n", err)
			os.Exit(1)
		}
		time.Sleep(time.Second / 10)
		println("OK")
		time.Sleep(time.Second)
	}
}
