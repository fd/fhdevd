package main

import (
	"fmt"
	"os"
	"time"

	"github.com/limbo-services/proc"
	"golang.org/x/net/context"
)

func NewWatcher(file string) proc.Runner {
	return func(ctx context.Context) <-chan error {
		out := make(chan error)
		go func() {
			defer close(out)

			fi, err := os.Stat(file)

			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					newFi, newErr := os.Stat(file)
					if err != nil {
						if newErr == nil {
							fmt.Printf("[%s]: error disapeared (was: %s)\n", file, err)
							return
						}
						if err.Error() != newErr.Error() {
							fmt.Printf("[%s]: error changed (was: %s; became: %s)\n", file, err, newErr)
							return
						}
					}
					if fi != nil {
						if newFi == nil {
							fmt.Printf("[%s]: file info disapeared\n", file)
							return
						}
						if newFi.ModTime().Unix() != fi.ModTime().Unix() {
							fmt.Printf("[%s]: mod time changed (was: %d; became: %d)\n", file, fi.ModTime().Unix(), newFi.ModTime().Unix())
							return
						}
						if newFi.Mode() != fi.Mode() {
							fmt.Printf("[%s]: mode changed (was: %s; became: %s)\n", file, fi.Mode(), newFi.Mode())
							return
						}
						if newFi.Size() != fi.Size() {
							fmt.Printf("[%s]: size changed (was: %d; became: %d)\n", file, fi.Size(), newFi.Size())
							return
						}
					}

				}
			}

		}()
		return out
	}
}
