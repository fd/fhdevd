package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

type Bind struct {
	Addr string
	Host string
}

func discover() <-chan Bind {
	out := make(chan Bind, 1)
	go func() {
		select {
		case b := <-discoverDNSDock():
			out <- b
		case b := <-discoverHeroku():
			out <- b
		case b := <-discoverLocal():
			out <- b
		case b := <-discoverDefault():
			out <- b
		}
	}()
	return out
}

func discoverDNSDock() <-chan Bind {
	out := make(chan Bind, 1)
	go func() {
		if os.Getenv("USE_DNSDOCK") != "true" {
			return
		}

		resp, err := http.Get("http://dnsdock.docker/services/" + os.Getenv("HOSTNAME"))
		if err != nil {
			return
		}

		if resp.StatusCode != 200 {
			return
		}
		defer resp.Body.Close()
		defer io.Copy(ioutil.Discard, resp.Body)

		var info struct {
			Name  string
			Image string
		}

		err = json.NewDecoder(resp.Body).Decode(&info)
		if err != nil {
			return
		}

		out <- Bind{
			Addr: ":80",
			Host: info.Name + "." + info.Image + ".docker",
		}
	}()
	return out
}

func discoverLocal() <-chan Bind {
	out := make(chan Bind, 1)
	go func() {
		if os.Getenv("USE_DNSDOCK") == "true" {
			return
		}
		if os.Getenv("DYNO") != "" {
			return
		}
		var port = os.Getenv("PORT")
		if port == "" {
			return
		}

		out <- Bind{
			Addr: ":" + port,
			Host: "localhost:" + port,
		}
	}()
	return out
}

func discoverHeroku() <-chan Bind {
	out := make(chan Bind, 1)
	go func() {
		if os.Getenv("USE_DNSDOCK") == "true" {
			return
		}
		if os.Getenv("DYNO") == "" {
			return
		}
		var port = os.Getenv("PORT")
		if port == "" {
			return
		}

		out <- Bind{
			Addr: ":" + port,
			Host: "0.0.0.0:" + port,
		}
	}()
	return out
}

func discoverDefault() <-chan Bind {
	out := make(chan Bind, 1)
	go func() {
		time.Sleep(2 * time.Second)

		out <- Bind{
			Addr: ":3080",
			Host: "localhost:3080",
		}
	}()
	return out
}
