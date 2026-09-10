package main

import (
	"os"
	"testing"
)

var pixivCacheTest = PixivCache{}

func TestRootMkdirOverwrite(t *testing.T) {
	err := pixivCacheTest.Root.Mkdir("testdir", 0o777)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}
	err = pixivCacheTest.Root.Mkdir("testdir", 0o777)
	if err != nil {
		t.Log(os.IsExist(err))
		t.Error(err)
		t.FailNow()
	}
}

func TestRootCreate(t *testing.T) {
	_, err := pixivCacheTest.Root.Create("noExistFolder/test.txt")
	if err != nil {
		t.Error(err)
		t.FailNow() // failed
	}
}

func TestRootFetch(t *testing.T) {
	pixivCacheTest.Init(`A:\Miuzarte\Pictures\BotpixivCacheTest`)
	datas, err := pixivCacheTest.Fetch("127308281")
	if err != nil {
		if os.IsNotExist(err) {
			t.Log("Cache not found")
		} else {
			t.Fatal(err)
		}
	}
	for i, data := range datas {
		t.Logf("%d: %d bytes", i, len(data))
	}
}

func TestRootStat(t *testing.T) {
	pixivCacheTest.Init(`A:\Miuzarte\Pictures\BotpixivCacheTest`)
	fi, err := pixivCacheTest.Root.Stat("127308281")
	if err != nil {
		if os.IsNotExist(err) {
			t.Log("Cache not found")
		} else {
			t.Error(err)
			t.FailNow()
		}
	}
	t.Log(fi.Name())
	t.Log(fi.Size())
	t.Log(fi.IsDir())
}

func TestReadDir(t *testing.T) {
	dirEnts, err := os.ReadDir(`A:\Miuzarte\Pictures\BotpixivCacheTest\127308281`)
	if err != nil {
		t.Fatal(err)
	}
	for i, de := range dirEnts {
		t.Logf("%d: %s, %v", i, de.Name(), de.IsDir())
	}
}
