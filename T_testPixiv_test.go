package main

import (
	"fmt"
	"os"
	"testing"
)

func TestPixivDownloadBytes(t *testing.T) {
	const id = 127122252
	datas, err := pixivClient.DownloadBytes(id)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	for i, data := range datas {
		fname := fmt.Sprintf("%d-%d.jpg", id, i+1)
		fs, err := os.Create(fname)
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
		fs.Write(data)
		err = fs.Close()
		if err != nil {
			t.Error(err)
			t.FailNow()
		}
	}
}
