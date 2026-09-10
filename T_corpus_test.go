package main

import "testing"

func TestCorpus(t *testing.T) {
	InitBot()
	onebot.Run()
	initCorpus()
	for i, corpus := range corpuses {
		t.Logf("%d: %+v\n", i, corpus)
	}
	onebot.Stop()
}

func TestTypeSwitch(t *testing.T) {
	m := make(map[string]any)
	m["string"] = "string"
	m["nil"] = nil
	s, ok := m["string"].(string)
	t.Logf("%v %t\n", s, ok)
	n, ok := m["nil"]
	t.Logf("%v, %t\n", n, ok)
	null, ok := m["null"]
	t.Logf("%v, %t\n", null, ok)

	s = m["string"].(string)
	t.Logf("%v\n", s)
	n = m["nil"]
	t.Logf("%v\n", n)
	null = m["null"]
	t.Logf("%v\n", null)

	nilString, ok := m["nil"].(string)
	t.Logf("%v, %t\n", nilString, ok)
	nullString, ok := m["null"].(string)
	t.Logf("%v, %t\n", nullString, ok)

	nilString = m["nil"].(string)
	t.Logf("%v\n", nilString)
	nullString = m["null"].(string)
	t.Logf("%v\n", nullString)
}
