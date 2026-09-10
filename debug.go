package main

import (
	"net/http"
	_ "net/http/pprof"

	env "NothingBot_v4/environment"
)

func init() {
	if env.NoBuild && !env.Testing {
		NoBuildPrintFile("debug.go")
		go func() {
			err := http.ListenAndServe("localhost:6060", nil)
			if err != nil {
				log.Warnf("failed to http.ListenAndServe: %v", err)
			}
		}()
	}
}
