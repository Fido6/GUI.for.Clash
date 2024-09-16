package bridge

import (
	"bytes"
	"crypto/tls"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) Requests(method string, url string, headers map[string]string, body string, options RequestOptions) HTTPResult {
	log.Printf("Requests: %v %v %v %v %v", method, url, headers, body, options)

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}

	req.Header = GetHeader(headers)

	client := withRequestOptionsClient(options)

	resp, err := client.Do(req)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}

	return HTTPResult{true, resp.StatusCode, resp.Header, string(b)}
}

func (a *App) Download(url string, path string, headers map[string]string, event string, options RequestOptions) HTTPResult {
	log.Printf("Download: %v %v %v, %v", url, path, headers, options)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}

	req.Header = GetHeader(headers)

	client := withRequestOptionsClient(options)

	resp, err := client.Do(req)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}
	defer resp.Body.Close()

	path = GetPath(path)

	err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}

	file, err := os.Create(path)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}
	defer file.Close()

	reader := io.TeeReader(resp.Body, &WriteTracker{Total: resp.ContentLength, ProgressChange: event, App: a})

	_, err = io.Copy(file, reader)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}

	return HTTPResult{true, resp.StatusCode, resp.Header, "Success"}
}

func (a *App) Upload(url string, path string, headers map[string]string, event string, options RequestOptions) HTTPResult {
	log.Printf("Upload: %v %v %v %v", url, path, headers, options)

	path = GetPath(path)

	file, err := os.Open(path)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}
	defer file.Close()

	fileStat, err := file.Stat()
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", path)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}

	reader := io.TeeReader(file, &WriteTracker{Total: fileStat.Size(), ProgressChange: event, App: a})

	_, err = io.Copy(part, reader)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}

	err = writer.Close()
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}

	req.Header = GetHeader(headers)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := withRequestOptionsClient(options)

	resp, err := client.Do(req)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return HTTPResult{false, 500, nil, err.Error()}
	}

	return HTTPResult{true, resp.StatusCode, resp.Header, string(b)}
}

func (wt *WriteTracker) Write(p []byte) (n int, err error) {
	wt.Progress += int64(len(p))
	if wt.ProgressChange != "" {
		wt.App.Ctx.EmitEvent(wt.ProgressChange, wt.Progress, wt.Total)
	}
	return
}

func withRequestOptionsClient(options RequestOptions) *http.Client {
	return &http.Client{
		Timeout: GetTimeout(options.Timeout),
		Transport: &http.Transport{
			Proxy: GetProxy(options.Proxy),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: options.Insecure,
			},
		},
	}
}
