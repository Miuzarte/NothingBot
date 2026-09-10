package main

import (
	"net/http"
	_ "net/http/pprof"

	env "NothingBot_v4/environment"
	"NothingBot_v4/logger"
)

// 本文件的日志 scope
var logDebug = logger.New("debug")

func init() {
	if env.NoBuild && !env.Testing {
		NoBuildPrintFile("debug.go")
		go func() {
			err := http.ListenAndServe("localhost:6060", nil)
			if err != nil {
				logDebug.Warn().
					Err(err).
					Msg("failed to http.ListenAndServe")
			}
		}()
	}
}
