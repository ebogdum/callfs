package main

import (
	"net/http"
	"net/http/pprof"
	"runtime"
)

const (
	profilingMutexFraction = 5
	profilingBlockRateNs   = 10000
)

func mountProfiling(mux *http.ServeMux) {
	runtime.SetMutexProfileFraction(profilingMutexFraction)
	runtime.SetBlockProfileRate(profilingBlockRateNs)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
