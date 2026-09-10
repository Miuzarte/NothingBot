package main

import (
	"fmt"
	"os"
	"testing"
)

func TestTiebaParse(t *testing.T) {
	tieba, err := os.ReadFile(`B:\Programming\Go\NothingBot_v4\tieba.json`)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	tp, err := unmarshalTiebaPost(string(tieba))
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	reply, err := tp.Format(982809597, "Miuzarte")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	for _, r := range reply {
		fmt.Println(r.String())
		fmt.Println()
	}
}
