package ocrspace

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const OCRSPACE_API_KEY = `68b47bddcb88957` // TODO: move to config struct

const (
	OCRSPACE_IMAGE_API    = `https://api.ocr.space/parse/image`    // POST
	OCRSPACE_IMAGEURL_API = `https://api.ocr.space/parse/imageurl` // GET
)

/*
{
  "ParsedResults": [
    {
      "TextOverlay": {
        "Lines": [],
        "HasOverlay": false,
        "Message": "Text overlay is not provided as it is not requested"
      },
      "TextOrientation": "0",
      "FileParseExitCode": 1,
      "ParsedText": "Solar cell\nArticle Talk\nFrom Wikipedia, the free encyclopedia\nFor convection cells on the Sun's surface, see Solar granule.\nA solar cell or photovoltaic cell (PV cell) is an electronic device that converts the energy of light directly\ninto electricity by means Of the photovoltaic effect. [II It is a form Of photoelectric cell, a device whose\nelectrical characteristics (such as current, voltage, or resistance) vary when exposed to light. Individual\nsolar cell devices are often the electrical building blocks of photovoltaic modules, known colloquially as\n\"solar panels\". The common single-junction silicon solar cell can produce a maximum Open-circuit voltage\nof approximately 0.5 to 0.6 volts.(2J\nPhotovoltaic cells may operate under sunlight or artificial light. In addition to producing energy, they can be\nused as a photodetector (for example infrared detectors), detecting light or other electromagnetic radiation\nnear the visible range, or measuring light intensity.\nRead\nEdit\n75 languages v\nView history Tools v\n",
      "ErrorMessage": "",
      "ErrorDetails": ""
    }
  ],
  "OCRExitCode": 1,
  "IsErroredOnProcessing": false,
  "ProcessingTimeInMilliseconds": "1359",
  "SearchablePDFURL": "Searchable PDF not generated as it was not requested."
}
*/

type OcrSpaceResponse struct {
	ParsedResults                []OcrSpaceParsedResult `json:"ParsedResults"`                // 解析结果数组，每个元素对应一次识别的结果
	OCRExitCode                  int                    `json:"OCRExitCode"`                  // 全局处理退出码（1=成功，其他代表异常/部分成功）
	IsErroredOnProcessing        bool                   `json:"IsErroredOnProcessing"`        // 是否在处理过程中发生错误
	ProcessingTimeInMilliseconds string                 `json:"ProcessingTimeInMilliseconds"` // 整体处理耗时（毫秒，字符串）
	SearchablePDFURL             string                 `json:"SearchablePDFURL"`             // 可检索 PDF 的下载链接（未请求时为提示文本）
}

type OcrSpaceParsedResult struct {
	TextOverlay       OcrSpaceTextOverlay `json:"TextOverlay"`       // 文本覆盖层信息（行/位置等），未请求时通常为空
	TextOrientation   string              `json:"TextOrientation"`   // 文本方向（角度，字符串表示，如 "0"）
	FileParseExitCode int                 `json:"FileParseExitCode"` // 单个文件/页的解析退出码（1=成功）
	ParsedText        string              `json:"ParsedText"`        // 识别出的纯文本内容（包含换行）
	ErrorMessage      string              `json:"ErrorMessage"`      // 错误消息（若有）
	ErrorDetails      string              `json:"ErrorDetails"`      // 错误详情（若有）
}

type OcrSpaceTextOverlay struct {
	// Lines []any `json:"Lines"` // 未使用
	HasOverlay bool   `json:"HasOverlay"` // 是否包含覆盖层（识别出的行/框位置信息）
	Message    string `json:"Message"`    // 说明信息（例如未请求覆盖层时的提示）
}

func (resp *OcrSpaceResponse) Error() (string, string) {
	if resp == nil || len(resp.ParsedResults) == 0 {
		return "", ""
	}
	return resp.ParsedResults[0].ErrorMessage, resp.ParsedResults[0].ErrorDetails
}

func (resp *OcrSpaceResponse) Texts() []string {
	if resp == nil || len(resp.ParsedResults) == 0 {
		return nil
	}
	parsedText := strings.TrimSpace(resp.ParsedResults[0].ParsedText)
	parsedText = strings.ReplaceAll(parsedText, "\r\n", "\n") // engine1 gives \r\n, engine2 gives \n
	texts := strings.Split(parsedText, "\n")
	// 移除空行
	i := 0
	for _, text := range texts {
		text = strings.TrimSpace(text)
		if text != "" {
			texts[i] = text
			i++
		}
	}
	return texts[:i]
}

type Config struct {
	ApiKey string // actually set in header

	Language                     *string
	IsOverlayRequired            *bool
	Filetype                     *string
	DetectOrientation            *bool
	IsCreateSearchablePdf        *bool
	IsSearchablePdfHideTextLayer *bool
	Scale                        *bool
	IsTable                      *bool
	OCREngine                    *int
}

type configFunc func(p *Config) error

var DefaultConfig, _ = NewConfig(
	WithApiKey("helloworld"),
	WithLanguage("auto"),
	WithOCREngine(2),
)

func NewConfig(params ...configFunc) (*Config, error) {
	p := &Config{
		ApiKey: "helloworld",
	}
	for _, fn := range params {
		if err := fn(p); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// Do performs the OCR request to ocr.space with the given file/input and parameters.
// About the `file` parameter, see [writeInputPart].
func Do(input any, config *Config) (resp *OcrSpaceResponse, err error) {
	return DoWithContext(context.Background(), input, config)
}

func DoWithContext(ctx context.Context, input any, config *Config) (*OcrSpaceResponse, error) {
	if config == nil {
		config = DefaultConfig
	}

	// Build x-www-form-urlencoded form
	form := url.Values{}
	if err := writeInputFields(form, input); err != nil {
		return nil, err
	}
	if err := writeOptionalFieldsForm(form, config); err != nil {
		return nil, err
	}
	body := bytes.NewBufferString(form.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, OCRSPACE_IMAGE_API, body)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("apikey", config.ApiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 60 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode/100 != 2 {
		b, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("ocr.space bad status %d: %s", res.StatusCode, string(b))
	}

	out := &OcrSpaceResponse{}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

// writeInputFields 将输入写入表单字段：
// - http/https 字符串 -> url 字段
// - data:... 字符串 -> base64Image 字段
// - 其他字符串 -> 当作本地文件读取并 base64 编码
// - []byte / io.Reader -> 直接 base64 编码
func writeInputFields(form url.Values, input any) error {
	switch v := input.(type) {
	case string:
		s := strings.TrimSpace(v)
		lower := strings.ToLower(s)
		switch {
		case strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://"):
			form.Set("url", s)
			return nil
		case strings.HasPrefix(lower, "data:"):
			form.Set("base64Image", s)
			return nil
		default:
			data, ct, err := readAllAndDetect(s)
			if err != nil {
				return err
			}
			form.Set("base64Image", makeDataURL(ct, data))
			return nil
		}
	case []byte:
		ct := http.DetectContentType(v)
		form.Set("base64Image", makeDataURL(ct, v))
		return nil
	case io.Reader:
		b, err := io.ReadAll(v)
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}
		ct := http.DetectContentType(b)
		form.Set("base64Image", makeDataURL(ct, b))
		return nil
	default:
		return fmt.Errorf("unsupported input type %T", input)
	}
}

func writeOptionalFieldsForm(form url.Values, p *Config) error {
	writeBool := func(name string, v *bool) {
		if v == nil {
			return
		}
		if *v {
			form.Set(name, "true")
		} else {
			form.Set(name, "false")
		}
	}
	writeString := func(name string, v *string) {
		if v == nil || *v == "" {
			return
		}
		form.Set(name, *v)
	}
	writeInt := func(name string, v *int) {
		if v == nil {
			return
		}
		form.Set(name, fmt.Sprintf("%d", *v))
	}

	writeString("language", p.Language)
	writeBool("isOverlayRequired", p.IsOverlayRequired)
	writeString("filetype", p.Filetype)
	writeBool("detectOrientation", p.DetectOrientation)
	writeBool("isCreateSearchablePdf", p.IsCreateSearchablePdf)
	writeBool("isSearchablePdfHideTextLayer", p.IsSearchablePdfHideTextLayer)
	writeBool("scale", p.Scale)
	writeBool("isTable", p.IsTable)
	writeInt("OCREngine", p.OCREngine)
	return nil
}

// readAllAndDetect 读取本地文件并返回数据与内容类型
func readAllAndDetect(path string) ([]byte, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, "", fmt.Errorf("read file: %w", err)
	}
	ct := http.DetectContentType(b)
	return b, ct, nil
}

// makeDataURL 构造 data: URL（带 mime 与 base64），多数情况下更兼容
func makeDataURL(contentType string, data []byte) string {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
}
