package main

import (
	"fmt"
	"net/http"
	"os"
	"syscall"

	"github.com/limbo-services/proc"
	"golang.org/x/net/context"
)

var bind = <-discover()

func main() {
	fmt.Printf("listening on: http://%s/\n", bind.Host)

	var handler SwappingHandler

	errs := proc.Run(context.Background(),
		proc.TerminateOnSignal(os.Interrupt, syscall.SIGTERM),
		proc.ServeHTTP(bind.Addr, &http.Server{Handler: &handler}),
		NewBuilder(&handler))

	exitcode := 0
	for err := range errs {
		if err == context.Canceled {
			continue
		}
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		exitcode = 1
	}
	os.Exit(exitcode)
}
