package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/limbo-services/proc"
	"golang.org/x/net/context"
)

type SwappingHandler struct {
	handler http.Handler
	mtx     sync.RWMutex
}

func (h *SwappingHandler) Set(newHandler http.Handler) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.handler = newHandler
}

func (h *SwappingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mtx.RLock()
	handler := h.handler
	h.mtx.RUnlock()

	if handler == nil {
		handler = http.NotFoundHandler()
	}

	handler.ServeHTTP(w, r)
}

func NewBuilder(h *SwappingHandler) proc.Runner {
	return func(ctx context.Context) <-chan error {
		out := make(chan error)
		go func() {
			defer close(out)

			var errs <-chan error

		LOOP:
			for {
				if errs == nil {
					errs = NewHandler(h)(ctx)
				}

				select {

				case <-ctx.Done():
					break LOOP

				case err, ok := <-errs:
					if err != nil {
						out <- err
					}
					if !ok {
						errs = nil
						fmt.Println("reloading...")
					}

				}
			}

			if errs != nil {
				for err := range errs {
					out <- err
				}
			}
		}()
		return out
	}
}

func NewHandler(h *SwappingHandler) proc.Runner {
	return func(ctx context.Context) <-chan error {
		out := make(chan error)
		go func() {
			defer close(out)

			var runners = []proc.Runner{
				NewWatcher(".env"),
				NewWatcher("Procfile"),
			}

			handler, watched := newHandler()
			for _, f := range watched {
				runners = append(runners, NewWatcher(f))
			}

			h.Set(handler)

			for err := range proc.Run(ctx, runners...) {
				out <- err
			}
		}()
		return out
	}
}
