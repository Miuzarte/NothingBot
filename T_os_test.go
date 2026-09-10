package main

import (
	"os"
	"testing"
)

func TestOsReadFileCap(t *testing.T) {
	data, err := os.ReadFile(`a:\Miuzarte\Pictures\BotEHentaiCache\3138775\1`)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("len(data) = %d", len(data))
	t.Logf("cap(data) = %d", cap(data))
}
