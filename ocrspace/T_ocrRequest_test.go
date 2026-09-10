package ocrspace

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"testing"
)

func TestOcrRequest(t *testing.T) {
	// https://api.ocr.space/parse/imageurl?apikey=helloworld&OCREngine=2&url=https://dl.a9t9.com/ocr/solarcell.jpg
	form := url.Values{
		"OCREngine": {"2"},
		"language":  {"auto"},
		"url":       {"https://dl.a9t9.com/ocr/solarcell.jpg"},
		"filetype":  {"jpg"},
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.ocr.space/parse/image", bytes.NewReader([]byte(form.Encode())))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("apikey", "helloworld")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("响应: %s", string(body))
}
