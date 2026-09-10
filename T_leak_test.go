package main

import (
	"runtime"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestLeak(t *testing.T) {
	defer func() {
		t.Logf("runtime.NumGoroutine(): %d", runtime.NumGoroutine())
		goleak.VerifyNone(t)
	}()
	InitBot().Run()
	AfterInit()
	// _, _ = EHentai.PostGalleryMetadata(t.Context(), EHentai.GIdList{3138775, "30b0285f9b"})
	time.Sleep(10 * time.Second)
	onebot.Stop()
	AtExit()
}
