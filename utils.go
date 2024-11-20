package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

var p *tea.Program

type progressWriter struct {
	total      int
	downloaded int
	file       *os.File
	reader     io.Reader
	onProgress func(float64)
}

func (pw *progressWriter) Start() {
	_, err := io.Copy(pw.file, io.TeeReader(pw.reader, pw))
	if err != nil {
		p.Send(progressErrMsg{err})
	}
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	pw.downloaded += len(p)
	if pw.total > 0 && pw.onProgress != nil {
		pw.onProgress(float64(pw.downloaded) / float64(pw.total))
	}
	return len(p), nil
}

type progressMsg float64

type progressErrMsg struct {
	err error
}

func (e progressErrMsg) Error() string {
	return e.err.Error()
}

func acceptEULA() error {
	eulaPath := filepath.Join(config.ServerPath, "eula.txt")
	content := "eula=true"
	if err := os.WriteFile(eulaPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write eula.txt: %v", err)
	}
	log.Println("By using this configuration, you are indicating your agreement to Minecraft's EULA.")
	log.Println("See https://www.minecraft.net/en-us/eula for more information.")
	log.Println("If you do not agree to the EULA above, please stop the server.")
	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func fetchJSON(url string, target interface{}) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bad status: %s, body: %s", resp.Status, body)
	}

	decoder := json.NewDecoder(resp.Body)
	return decoder.Decode(target)
}

func downloadFile(url string, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bad status: %s, body: %s", resp.Status, body)
	}

	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	pw := &progressWriter{
		total:  int(resp.ContentLength),
		file:   file,
		reader: resp.Body,
		onProgress: func(ratio float64) {
			if p != nil {
				p.Send(progressMsg(ratio))
			}
		},
	}

	m := newDownloadModel(pw)
	p = tea.NewProgram(m)

	go pw.Start()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running progress display: %v", err)
	}

	return nil
}

func verifyMD5(filepath string, expectedMD5 string) (bool, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return false, err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, err
	}

	actualMD5 := hex.EncodeToString(hash.Sum(nil))
	return actualMD5 == expectedMD5, nil
}
