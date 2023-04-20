package main

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
)

type VNCInfo struct {
	Username string
	Password string
	ClientName string
	Width int
	Height int
}

func doScreenshot(ip, port, username, password string) (*VNCInfo, error) {
	file := fmt.Sprintf("./screenshots/%s_%s.jpeg", ip, port)
	cmd := exec.Command(CFGVNCScreenshotBin, CFGVNCTimeout, ip, port, file, username, password)
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.New(strings.TrimSpace(string(out)))
	}
	split := strings.Split(string(out), "\n")

	if split[0] == "0" {
		username = ""
		password = ""
	} else if split[0] == "1" {
		username = ""
	}

	w, err := strconv.Atoi(split[1])
	if err != nil {
		return nil, err
	}
	h, err := strconv.Atoi(split[2])
	if err != nil {
		return nil, err
	}

	return &VNCInfo{
		Username: username,
		Password: password,
		ClientName: split[3],
		Width: w,
		Height: h,
	}, nil
}

func VNCScreenshot(ip, port, username, password string) (*VNCInfo, error) {
	if password != "" {
		return doScreenshot(ip, port, username, password)
	}

	for _, s := range CFGPasswords {
		if username == "" {
			username = "admin"
		}
		info, err := doScreenshot(ip, port, username, s)
		if err != nil && err.Error() != "Auth failed" {
			return nil, err
		} else if err != nil {
			continue
		}
		return info, nil
	}
	return nil, errors.New("Could not screenshot VNC")
}

var vLimit = make(chan struct{}, CFGMaxVNCConns)
func vAcquire() { vLimit <- struct{}{} }
func vRelease() { <-vLimit }

func AddVNC(ip, port, username, password string, db *DB) error {
	vAcquire()
	info, err := VNCScreenshot(ip, port, username, password)
	vRelease()
	if err != nil {
		log.Printf("%s:%s %s", ip, port, err.Error())
		return err
	}
	ocr := OCR(fmt.Sprintf("./screenshots/%s_%s.jpeg", ip, port))
	err = db.AddService(ip, port, ocr, info)
	if err != nil {
		log.Printf("%s:%s %s", ip, port, err.Error())
		return err
	}
	log.Printf("%s:%s Added VNC Successfully!", ip, port)
	return nil
}
