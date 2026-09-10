package main

import (
	"bytes"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	env "NothingBot_v4/environment"
	"NothingBot_v4/utils"

	"codeberg.org/go-pdf/fpdf"
)

func NoBuildPrintFile(s string) {
	if env.NoBuild {
		println(utils.Hyperlink(s))
	}
}

func NoBuildPrintln(s string) {
	if env.NoBuild {
		println(s)
	}
}

type interger interface {
	~int | ~int32 | ~int64 |
		~uint | ~uint32 | ~uint64
}

type number interface {
	interger | ~float32 | ~float64
}

func Itoa[T number](i T) string {
	return strconv.FormatInt(int64(i), 10)
}

var PdfInit = fpdf.InitType{
	OrientationStr: "P",
	UnitStr:        "pt",
	SizeStr:        "A4",
	FontDirStr:     ".",
}

type ImagePdf struct {
	*fpdf.Fpdf
}

func NewImagePdf() *ImagePdf {
	pdf := fpdf.NewCustom(&PdfInit)
	return &ImagePdf{Fpdf: pdf}
}

func (ip *ImagePdf) AddImage(img []byte, bounds image.Rectangle, link string) {
	ip.AddPageFormat("P", fpdf.SizeType{
		Wd: float64(bounds.Dx()),
		Ht: float64(bounds.Dy()),
	})
	imgName := Itoa(ip.PageNo())
	_ = ip.RegisterImageOptionsReader(
		imgName,
		fpdf.ImageOptions{ImageType: "jpg"},
		bytes.NewReader(img),
	)
	ip.ImageOptions(
		imgName,
		0, 0, -1, -1,
		false,
		fpdf.ImageOptions{ImageType: "jpg"},
		0, link,
	)
}

func IsAlphanumeric(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

// CreateLogFile 创建日志文件
func CreateLogFile(dir string) *SafeFile {
	if dir == "" {
		return nil
	}
	fi, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(dir, 0o755)
			if err != nil {
				log.Fatal("failed to create log directory: ", err)
			}
		} else {
			log.Fatal("failed to access log directory: ", err)
		}
	}
	if !fi.IsDir() {
		log.Fatal(dir, " is not a directory")
	}

	fileName := time.Now().Format("20060102150405") + ".log"
	path := filepath.Join(dir, fileName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o666)
	if err != nil {
		log.Fatal("failed to create log file: ", err)
	}
	log.Info("saving log to: ", path)
	return NewSafeFile(f)
}

type SafeFile struct {
	file *os.File
	mu   sync.Mutex
}

func NewSafeFile(f *os.File) *SafeFile {
	return &SafeFile{
		file: f,
	}
}

func (f *SafeFile) Write(p []byte) (n int, err error) {
	if f.file == nil {
		return 0, errors.New("file is closed")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Write(p)
}

func (f *SafeFile) Close() error {
	if f.file == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.file.Close()
}
