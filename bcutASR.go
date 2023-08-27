package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	API_REQ_UPLOAD    = "https://member.bilibili.com/x/bcut/rubick-interface/resource/create"          // 申请上传
	API_COMMIT_UPLOAD = "https://member.bilibili.com/x/bcut/rubick-interface/resource/create/complete" // 提交上传
	API_CREATE_TASK   = "https://member.bilibili.com/x/bcut/rubick-interface/task"                     // 创建任务
	API_QUERY_RESULT  = "https://member.bilibili.com/x/bcut/rubick-interface/task/result"              // 查询结果
)

var (
	SUPPORT_SOUND_FORMAT = []string{"flac", "aac", "m4a", "mp3", "wav"}
	INFILE_FMT           = []string{"flac", "aac", "m4a", "mp3", "wav"}
	OUTFILE_FMT          = []string{"srt", "json", "lrc", "txt"}
)

type BcutASR struct {
	session     *http.Client
	soundName   string
	soundBin    []byte
	soundFmt    string
	inBossKey   string
	resourceID  string
	uploadID    string
	uploadURLs  []string
	perSize     int
	clips       int
	etags       []string
	downloadURL string
	taskID      string
}

type ASRDataWords struct {
	Label      string `json:"label"`
	StartTime  int    `json:"start_time"`
	EndTime    int    `json:"end_time"`
	Confidence int    `json:"confidence"`
}

type ASRDataSeg struct {
	StartTime  int            `json:"start_time"`
	EndTime    int            `json:"end_time"`
	Transcript string         `json:"transcript"`
	Words      []ASRDataWords `json:"words"`
	Confidence int            `json:"confidence"`
}

type ASRData struct {
	Utterances []ASRDataSeg `json:"utterances"`
	Version    string       `json:"version"`
}

type ResourceCreateRspSchema struct {
	ResourceID string   `json:"resource_id"`
	Title      string   `json:"title"`
	Type       int      `json:"type"`
	InBossKey  string   `json:"in_boss_key"`
	Size       int      `json:"size"`
	UploadURLs []string `json:"upload_urls"`
	UploadID   string   `json:"upload_id"`
	PerSize    int      `json:"per_size"`
}

type ResourceCompleteRspSchema struct {
	ResourceID  string `json:"resource_id"`
	DownloadURL string `json:"download_url"`
}

type TaskCreateRspSchema struct {
	Resource string `json:"resource"`
	Result   string `json:"result"`
	TaskID   string `json:"task_id"`
}

type ResultRspSchema struct {
	TaskID string `json:"task_id"`
	Result string `json:"result"`
	Remark string `json:"remark"`
	State  int    `json:"state"`
}

func NewBcutASR() *BcutASR {
	return &BcutASR{
		session: &http.Client{},
	}
}

func (asr *BcutASR) SetData(file string, rawdata []byte, dataFmt string) error {
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			return err
		}

		buf := make([]byte, stat.Size())
		_, err = f.Read(buf)
		if err != nil {
			return err
		}

		asr.soundBin = buf
		asr.soundName = filepath.Base(file)
		asr.soundFmt = filepath.Ext(file)[1:]
	} else if len(rawdata) > 0 {
		asr.soundBin = rawdata
		asr.soundName = fmt.Sprintf("%d.%s", time.Now().Unix(), dataFmt)
		asr.soundFmt = dataFmt
	} else {
		return errors.New("none set data")
	}

	if !contains(SUPPORT_SOUND_FORMAT, asr.soundFmt) {
		return errors.New("format is not supported")
	}

	log.Infof("加载文件成功: %s", asr.soundName)
	return nil
}

func (asr *BcutASR) Upload() error {
	if len(asr.soundBin) == 0 || asr.soundFmt == "" {
		return errors.New("none set data")
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	_ = writer.WriteField("type", "2")
	_ = writer.WriteField("name", asr.soundName)
	_ = writer.WriteField("size", strconv.Itoa(len(asr.soundBin)))
	_ = writer.WriteField("resource_file_type", asr.soundFmt)
	_ = writer.WriteField("model_id", "7")

	err := writer.Close()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", API_REQ_UPLOAD, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := asr.session.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var respData struct {
		Code int                     `json:"code"`
		Data ResourceCreateRspSchema `json:"data"`
	}

	err = json.Unmarshal(respBody, &respData)
	if err != nil {
		return err
	}

	if respData.Code != 0 {
		return fmt.Errorf("%d: %v", respData.Code, respData.Data)
	}

	asr.inBossKey = respData.Data.InBossKey
	asr.resourceID = respData.Data.ResourceID
	asr.uploadID = respData.Data.UploadID
	asr.uploadURLs = respData.Data.UploadURLs
	asr.perSize = respData.Data.PerSize
	asr.clips = len(respData.Data.UploadURLs)

	log.Infof("申请上传成功, 总计大小%dKB, %d分片, 分片大小%dKB: %s", respData.Data.Size/1024, asr.clips, respData.Data.PerSize/1024, asr.inBossKey)

	err = asr.uploadPart()
	if err != nil {
		return err
	}

	err = asr.commitUpload()
	if err != nil {
		return err
	}

	return nil
}

func (asr *BcutASR) uploadPart() error {
	for clip := 0; clip < asr.clips; clip++ {
		startRange := clip * asr.perSize
		endRange := (clip + 1) * asr.perSize

		log.Infof("开始上传分片%d: %d-%d", clip, startRange, endRange)

		if endRange >= len(asr.soundBin) {
			endRange = len(asr.soundBin) - 1
		}

		req, err := http.NewRequest("PUT", asr.uploadURLs[clip], bytes.NewReader(asr.soundBin[startRange:endRange]))
		if err != nil {
			return err
		}

		resp, err := asr.session.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		etag := resp.Header.Get("Etag")
		asr.etags = append(asr.etags, etag)

		log.Infof("分片%d上传成功: %s", clip, etag)
	}

	return nil
}

func (asr *BcutASR) commitUpload() error {
	data := map[string]string{
		"in_boss_key": asr.inBossKey,
		"resource_id": asr.resourceID,
		"etags":       strings.Join(asr.etags, ","),
		"upload_id":   asr.uploadID,
		"model_id":    "7",
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	resp, err := asr.session.Post(API_COMMIT_UPLOAD, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var respData struct {
		Code int                       `json:"code"`
		Data ResourceCompleteRspSchema `json:"data"`
	}

	err = json.Unmarshal(respBody, &respData)
	if err != nil {
		return err
	}

	if respData.Code != 0 {
		return fmt.Errorf("%d: %s", respData.Code, respData.Data)
	}

	asr.downloadURL = respData.Data.DownloadURL

	log.Info("提交成功")
	return nil
}

func (asr *BcutASR) CreateTask() (string, error) {
	data := map[string]string{
		"resource": asr.downloadURL,
		"model_id": "7",
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	resp, err := asr.session.Post(API_CREATE_TASK, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var respData struct {
		Code int                 `json:"code"`
		Data TaskCreateRspSchema `json:"data"`
	}

	err = json.Unmarshal(respBody, &respData)
	if err != nil {
		return "", err
	}

	if respData.Code != 0 {
		return "", fmt.Errorf("%d: %s", respData.Code, respData.Data)
	}

	asr.taskID = respData.Data.TaskID

	log.Infof("任务已创建: %s", asr.taskID)
	return asr.taskID, nil
}

func (asr *BcutASR) Result(taskID string) (*ResultRspSchema, error) {
	resp, err := asr.session.Get(API_QUERY_RESULT + "?model_id=7&task_id=" + taskID)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var respData struct {
		Code int             `json:"code"`
		Data ResultRspSchema `json:"data"`
	}

	err = json.Unmarshal(respBody, &respData)
	if err != nil {
		return nil, err
	}

	if respData.Code != 0 {
		return nil, fmt.Errorf("%d: %v", respData.Code, respData.Data)
	}

	return &respData.Data, nil
}

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func ffmpegRender(mediaFile string) ([]byte, error) {
	cmd := exec.Command("ffmpeg", "-v", "warning", "-i", mediaFile, "-f", "adts", "-ac", "1", "pipe:")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func bcutASR(inPath string, outPath string, format string) {
	if inPath == "" {
		log.Error("输入文件错误")
	}

	infileStat, err := os.Stat(inPath)
	if err != nil {
		log.Error(err)
	}

	infileName := infileStat.Name()
	suffix := filepath.Ext(infileName)[1:]

	var infileData []byte
	var infileFmt string

	if contains(INFILE_FMT, suffix) {
		infileFmt = suffix
		infileData, err = ioutil.ReadFile(inPath)
		if err != nil {
			log.Error(err)
		}
	} else {
		log.Println("非标准音频文件, 尝试调用ffmpeg转码")
		infileData, err = ffmpegRender(inPath)
		if err != nil {
			log.Error("ffmpeg转码失败")
		}
		log.Info("ffmpeg转码完成")
		infileFmt = "aac"
	}

	var outfileData []byte

	if outPath == "" {
		if format == "" {
			format = "srt"
		}
	} else {
		outfileStat, err := os.Stat(outPath)
		if err != nil {
			log.Error(err)
		}

		outfileName := outfileStat.Name()
		if outfileName == "<stdout>" {
			if format == "" {
				format = "srt"
			}
		} else {
			suffix := filepath.Ext(outfileName)[1:]
			if !contains(OUTFILE_FMT, suffix) {
				log.Error("输出格式错误")
			}
			format = suffix
		}
	}

	asr := NewBcutASR()
	err = asr.SetData(inPath, infileData, infileFmt)
	if err != nil {
		log.Error(err)
	}

	err = asr.Upload()
	if err != nil {
		log.Error(err)
	}

	taskID, err := asr.CreateTask()
	if err != nil {
		log.Error(err)
	}

	for {
		taskResp, err := asr.Result(taskID)
		if err != nil {
			log.Error(err)
		}

		if taskResp.State == 0 {
			log.Info("等待识别开始")
		} else if taskResp.State == 1 {
			log.Infof("识别中-%s", taskResp.Remark)
		} else if taskResp.State == 3 {
			log.Errorf("识别失败-%s", taskResp.Remark)
		} else if taskResp.State == 4 {
			log.Info("识别成功")
			result := &ASRData{}
			err := json.Unmarshal([]byte(taskResp.Result), result)
			if err != nil {
				log.Error(err)
			}
			if !hasData(result) {
				log.Error("未识别到语音")
			}

			switch format {
			case "srt":
				outfileData = []byte(result.ToSrt())
			case "lrc":
				outfileData = []byte(result.ToLrc())
			case "json":
				outfileData, err = json.Marshal(result)
				if err != nil {
					log.Error(err)
				}
			case "txt":
				outfileData = []byte(result.ToTxt())
			default:
				log.Error("输出格式错误")
			}

			break
		}

		time.Sleep(1 * time.Second)
	}

	if outPath == "" {
		fmt.Println(string(outfileData))
	} else {
		err = ioutil.WriteFile(outPath, outfileData, 0644)
		if err != nil {
			log.Error(err)
		}
	}

	log.Info("转换成功")
}

func hasData(result *ASRData) bool {
	return len(result.Utterances) > 0
}

func (result *ASRData) ToTxt() string {
	var lines []string
	for _, seg := range result.Utterances {
		lines = append(lines, seg.Transcript)
	}
	return strings.Join(lines, "")
}

func (result *ASRData) ToSrt() string {
	var lines []string
	for i, seg := range result.Utterances {
		lines = append(lines, fmt.Sprintf("%d\n%s --> %s\n%s", i+1, toSrtTimestamp(seg.StartTime), toSrtTimestamp(seg.EndTime), seg.Transcript))
	}
	return strings.Join(lines, "")
}

func (result *ASRData) ToLrc() string {
	var lines []string
	for _, seg := range result.Utterances {
		lines = append(lines, fmt.Sprintf("[%s]%s", toLrcTimestamp(seg.StartTime), seg.Transcript))
	}
	return strings.Join(lines, "")
}

func toSrtTimestamp(ms int) string {
	h := ms / 3600000
	m := (ms / 60000) % 60
	s := (ms / 1000) % 60
	ms = ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func toLrcTimestamp(ms int) string {
	m := ms / 60000
	s := (ms / 1000) % 60
	ms = (ms % 1000) / 10
	return fmt.Sprintf("[%02d:%02d.%02d]", m, s, ms)
}

type ArgumentParser struct {
	prog        string
	description string
	epilog      string
	arguments   []*Argument
}

type Argument struct {
	name       string
	help       string
	required   bool
	defaultVal interface{}
	choices    []string
	value      interface{}
}

func NewArgumentParser(prog, description string) *ArgumentParser {
	return &ArgumentParser{
		prog:        prog,
		description: description,
	}
}

func (ap *ArgumentParser) AddArgument(name, help string, required bool, defaultVal interface{}, choices ...string) {
	ap.arguments = append(ap.arguments, &Argument{
		name:       name,
		help:       help,
		required:   required,
		defaultVal: defaultVal,
		choices:    choices,
	})
}

func (ap *ArgumentParser) ParseArgs() map[string]interface{} {
	args := make(map[string]interface{})

	for _, arg := range ap.arguments {
		if arg.required {
			fmt.Printf("%s: ", arg.name)
			input := ""
			fmt.Scanln(&input)
			args[arg.name] = input
		} else {
			args[arg.name] = arg.defaultVal
		}
	}

	return args
}
