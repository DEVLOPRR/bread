package utils

// #include <stdio.h>
// #include <stdlib.h>
import "C"

import (
	"bytes"
	"crypto/sha1"
	"debug/elf"
	"encoding/hex"
	"fmt"
	"github.com/manifoldco/promptui"
	"github.com/schollz/progressbar/v3"
	"io"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
)

type BinaryUrl struct {
	FileName string
	Url      string
}

func ShowSignature(filePath string) (error) {
	signingEntity, err := VerifySignature(filePath)
	if err != nil {
		return err
	}
	if signingEntity != nil {
		fmt.Println("AppImage signed by:")
		for _, v := range signingEntity.Identities {
			fmt.Println("\t", v.Name)
		}
	}
	return nil
}

func MakeApplicationsDirPath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}

	applicationsPath := filepath.Join(usr.HomeDir, "Applications")
	err = os.MkdirAll(applicationsPath, os.ModePerm)
	if err != nil {
		return "", err
	}
	return applicationsPath, nil
}

func DownloadAppImage(url string, filePath string) error {
	output, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return err
	}
	defer output.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	bar := progressbar.DefaultBytes(
		resp.ContentLength,
		"Downloading",
	)

	go func() {
		sigchan := make(chan os.Signal, 1)
		signal.Notify(sigchan, os.Interrupt)
		<-sigchan

		_ = resp.Body.Close()
		_ = output.Close()
		_ = os.Remove(filePath)

		os.Exit(0)
	}()

	_, err = io.Copy(io.MultiWriter(output, bar), resp.Body)
	return err
}

func MakeTargetFilePath(link *BinaryUrl) (string, error) {
	applicationsPath, err := MakeApplicationsDirPath()
	if err != nil {
		return "", err
	}

	filePath := filepath.Join(applicationsPath, link.FileName)
	return filePath, nil
}

func MakeTempFilePath(link *BinaryUrl) (string, error) {
	applicationsPath, err := MakeTempAppDirPath()
	if err != nil {
		return "", err
	}

	filePath := filepath.Join(applicationsPath, link.FileName)
	return filePath, nil
}

func MakeTempAppDirPath() (string, error) {
	TempApplicationDirPath, err := MakeApplicationsDirPath()
	if err != nil {
		return "", err
	}

	TempApplicationDirPath = filepath.Join(TempApplicationDirPath, "run-cache")
	err = os.MkdirAll(TempApplicationDirPath, os.ModePerm)
	if err != nil {
		return "", err
	}

	return TempApplicationDirPath, nil
}

func PromptBinarySelection(downloadLinks []BinaryUrl) (result *BinaryUrl, err error) {
	if len(downloadLinks) == 1 {
		return &downloadLinks[0], nil
	}

	prompt := promptui.Select{
		Label: "Select an AppImage to install",
		Items: downloadLinks,
		Templates: &promptui.SelectTemplates{
			Label:    "   {{ .FileName }}",
			Active:   "\U00002713 {{ .FileName }}",
			Inactive: "   {{ .FileName }}",
			Selected: "\U00002713 {{ .FileName }}"},
	}

	i, _, err := prompt.Run()
	if err != nil {
		return nil, err
	}

	return &downloadLinks[i], nil
}

func ReadUpdateInfo(appImagePath string) (string, error) {
	elfFile, err := elf.Open(appImagePath)
	if err != nil {
		panic("Unable to open target: \"" + appImagePath + "\"." + err.Error())
	}

	updInfo := elfFile.Section(".upd_info")
	if updInfo == nil {
		panic("Missing update section on target elf ")
	}
	sectionData, err := updInfo.Data()

	if err != nil {
		panic("Unable to parse update section: " + err.Error())
	}

	str_end := bytes.Index(sectionData, []byte("\000"))
	if str_end == -1 || str_end == 0 {
		return "", fmt.Errorf("No update information found in: " + appImagePath)
	}

	update_info := string(sectionData[:str_end])
	return update_info, nil
}

func GetFileSHA1(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}

	sha1Checksum := sha1.New()
	_, err = io.Copy(sha1Checksum, file)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sha1Checksum.Sum(nil)), nil
}

func IsAppImageFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}

	return isAppImage1File(file) || isAppImage2File(file)
}

func isAppImage2File(file *os.File) bool {
	return isElfFile(file) && fileHasBytesAt(file, []byte{0x41, 0x49, 0x02}, 8)
}

func isAppImage1File(file *os.File) bool {
	return isISO9660(file) || fileHasBytesAt(file, []byte{0x41, 0x49, 0x01}, 8)
}

func isElfFile(file *os.File) bool {
	return fileHasBytesAt(file, []byte{0x7f, 0x45, 0x4c, 0x46}, 0)
}

func isISO9660(file *os.File) bool {
	for _, offset := range []int64{32769, 34817, 36865} {
		if fileHasBytesAt(file, []byte{'C', 'D', '0', '0', '1'}, offset) {
			return true
		}
	}

	return false
}

func fileHasBytesAt(file *os.File, expectedBytes []byte, offset int64) bool {
	readBytes := make([]byte, len(expectedBytes))
	_, _ = file.Seek(offset, 0)
	_, _ = file.Read(readBytes)

	return bytes.Equal(readBytes, expectedBytes)
}
