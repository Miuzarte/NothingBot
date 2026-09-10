package main

import (
	"fmt"
	"os"
	"testing"
)

func TestTest(t *testing.T) {
	fmt.Println(os.Executable())
}

func TestMap(t *testing.T) {
	m := make(map[string]string)
	m["a"] = "a"
	delete(m, "a")
	delete(m, "b")
}

func TestRuneEmoji(t *testing.T) {
	const s = `👻✈️の🦸‍♂️💢🐯！🏴‍☠️の🏆🔫C210`
	for _, r := range s {
		fmt.Printf("%d: %c\n", r, r)
	}
}
