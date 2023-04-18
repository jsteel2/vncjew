package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"strings"
	"strconv"

	"golang.org/x/net/websocket"
)

var masscanCmd *exec.Cmd
var masscanStatus string
var vncAddURL string
var password string
var defaultArgs = []string{"--open", "--open-only", "-p5900-5910",
	"-oD", "/dev/stdout", "--banners", "--source-port", "31342",
	"--exclude", "10.0.0.0/8", "--exclude", "172.16.0.0/12",
	"--exclude", "192.168.0.0/16"}

func main() {
	user, err := user.Current()
	if err != nil || user.Uid != "0" {
		log.Fatal("Run as root!")
	}

	if len(os.Args) < 3 {
		log.Fatalf("Usage: %s <password> <server>", os.Args[0])
	}
	password = os.Args[1]

	iptables := exec.Command("iptables", "-A", "INPUT", "-p", "tcp", "--dport", "31342", "-j", "DROP")
	if err := iptables.Run(); err != nil {
		log.Fatal(err)
	}

	origin := &url.URL{Scheme: "http", Host: os.Args[2]}
	_url := &url.URL{Scheme: "ws", Host: os.Args[2], Path: "/api/client"}
	auth := base64.StdEncoding.EncodeToString([]byte("client:" + password))
	vncAddURL = fmt.Sprintf("http://%s/api/addvnc", os.Args[2])

	ws, err := websocket.DialConfig(&websocket.Config{
		Location: _url,
		Origin: origin,
		Version: websocket.ProtocolVersionHybi13,
		Header: http.Header{"Authorization": {"Basic " + auth}},
	})
	if err != nil {
		log.Fatal(err)
	}

	msg := make([]byte, 1024)
	for {
		n, err := ws.Read(msg)
		if err != nil {
			log.Fatal(err)
		}

		split := strings.Fields(string(msg[:n]))
		log.Println("Got", split)

		if len(split) < 1 {
			continue
		}

		switch split[0] {
		case "status": ws.Write([]byte(getStatus()))
		case "scan": ws.Write([]byte(masscan(split[1:])))
		case "stop": ws.Write([]byte(stop()))
		}
	}
}

func running() bool {
	return masscanCmd != nil && masscanCmd.ProcessState == nil
}

func readVNCs(from io.ReadCloser) {
	scanner := bufio.NewScanner(from)

	for scanner.Scan() {
		var data map[string]any
		err := json.Unmarshal([]byte(scanner.Text()), &data)
		if err != nil {
			log.Println("Error parsing json", scanner.Text())
			break
		}
		d := data["data"].(map[string]any)
		if data["rec_type"] != "banner" || d["service_name"] != "vnc" {
			continue
		}
		log.Println("Putting in", data["ip"], data["port"])
		v := url.Values{}
		v.Set("ip", data["ip"].(string))
		v.Set("port", strconv.Itoa(int(data["port"].(float64))))
		r, err := http.NewRequest("POST", vncAddURL, strings.NewReader(v.Encode()))

		if err != nil {
			log.Println("Failed to POST VNC", err)
			continue
		}
		r.SetBasicAuth("client", password)
		r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		client := &http.Client{}
		go func() {
			res, err := client.Do(r)
			if err != nil {
				log.Println("Failed to POST VNC", err)
				return
			}
			defer res.Body.Close()

			body, err := ioutil.ReadAll(res.Body)
			if err != nil {
				log.Println("Failed to POST VNC", err)
				return
			}
			fmt.Println("POST", data["ip"], data["port"], string(body))
		}()
	}
}

func readStatus(from io.ReadCloser) {
	scanner := bufio.NewScanner(from)
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		if atEOF && len(data) == 0 {
			return 0, nil, nil
		}

		if data[len(data) - 1] == '\n' {
			return len(data), nil, nil
		}

		if data[len(data) - 1] == '\r' {
			return len(data), data, nil
		}

		if atEOF {
			return len(data), data, nil
		}

		return
	})

	for scanner.Scan() {
		masscanStatus = scanner.Text()
	}
}

func getStatus() string {
	if !running() {
		return "Idling"
	}
	return masscanStatus
}

func masscan(args []string) string {
	if !running() {
		os.Remove("./paused.conf")

		masscanStatus = ""
		args = append(defaultArgs, args...)
		log.Println("started", args)
		masscanCmd = exec.Command("masscan", args...)

		stdout, err := masscanCmd.StdoutPipe()
		if err != nil {
			return "could not get stdout"
		}

		go readVNCs(stdout)

		stderr, err := masscanCmd.StderrPipe()
		if err != nil {
			return "could not get stderr"
		}

		go readStatus(stderr)

		err = masscanCmd.Start()
		if err != nil {
			return "could not start scan"
		}

		go func() {
			masscanCmd.Wait()
		}()

		return "started new scan"
	}
	return "already scanning!"
}

func stop() string {
	if !running() {
		return "already stopped"
	}
	err := masscanCmd.Process.Kill()
	if err != nil {
		return "could not stop scan"
	}
	return "stopped scan"
}
