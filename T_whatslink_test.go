package main

import (
	"io"
	"net/http"
	"testing"
)

func TestWhatsLink(t *testing.T) {
	req, err := http.NewRequest("GET", "https://httpbin.org/get", nil)
	if err != nil {
		t.Error(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Error(err)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
	}
	t.Log(string(b))

	const u = `https://whatslink.info/api/v1/link?url=magnet:?xt=urn:btih:0966F81337A95D0B5B1EB688C6ABFEB3DACCE808`
	req, err = http.NewRequest("GET", u, nil)
	if err != nil {
		t.Error(err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Error(err)
	}
	b, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Error(err)
	}
	t.Log(string(b))
}
