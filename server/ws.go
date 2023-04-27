package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"time"

	"golang.org/x/net/websocket"
)

type Client struct {
	Ws *websocket.Conn
	StatusChan chan string
	StartChan chan string
	StopChan chan string
	Ip string
}

type WSServ struct {
	Clients map[*Client]struct{}
	RangeChan chan int
	Db *DB
	Started bool
}

type Response struct {
	Client *Client
	Response string
}

func NewClient(ws *websocket.Conn) *Client {
	return &Client{
		Ws: ws,
		StatusChan: make(chan string),
		StartChan: make(chan string),
		StopChan: make(chan string),
		Ip: getIP(ws.Request()),
	}
}

func NewWSServ(db *DB) *WSServ {
	return &WSServ{
		Clients: make(map[*Client]struct{}),
		Db: db,
		Started: false,
	}
}

func (c *Client) ReadMSG() ([]string, error) {
	buf := make([]byte, 1024)
	n, err := c.Ws.Read(buf)
	if err != nil {
		return nil, err
	}
	var res []string
	err = json.Unmarshal(buf[:n], &res)
	return res, err
}

func (c *Client) WriteMSG(msg ...string) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = c.Ws.Write(b)
	return err
}

func (c *Client) Send(msg string, chn chan string) (string, error) {
	if err := c.WriteMSG(msg); err != nil {
		return "", err
	}
	res, ok := <-chn
	if !ok {
		return "", errors.New("Client channel closed")
	}
	return res, nil
}

func sendStatus(c *Client) (string, error) {
	return c.Send("status", c.StatusChan)
}

func sendStart(c *Client) (string, error) {
	return c.Send("start", c.StartChan)
}

func sendStop(c *Client) (string, error) {
	return c.Send("stop", c.StopChan)
}

func (s *WSServ) Send(fn func(c *Client) (string, error)) ([]Response) {
	res := make([]Response, 0, len(s.Clients))
	for c := range s.Clients {
		s, err := fn(c)
		if err != nil {
			s = err.Error()
		}
		res = append(res, Response{
			Client: c,
			Response: s,
		})
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Client.Ip < res[j].Client.Ip
	})
	return res
}

func (s *WSServ) SendStatus() ([]Response) {
	return s.Send(sendStatus)
}

func (s *WSServ) SendStart() ([]Response) {
	s.Started = true
	s.InitRanges()
	return s.Send(sendStart)
}

func (s *WSServ) SendStop() ([]Response) {
	s.Started = false
	return s.Send(sendStop)
}

func (s *WSServ) InitRanges() {
	arr := make([]int, 256)
	for i := range arr {
		arr[i] = i
	}
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(arr), func(i, j int) { arr[i], arr[j] = arr[j], arr[i] })
	s.RangeChan = make(chan int, len(arr))
	for _, e := range arr {
		s.RangeChan <- e
	}
	close(s.RangeChan)
}

func (s *WSServ) SendRange(c *Client) error {
	select {
	case x, ok := <-s.RangeChan:
		if ok {
			return c.WriteMSG("range", fmt.Sprintf("%d.0.0.0/8", x))
		}
		s.Started = false
		return c.WriteMSG("range", "stop")
	default:
		s.Started = false
		return c.WriteMSG("range", "stop")
	}
}

func (s *WSServ) SendVNC(ip, port string, c *Client) error {
	err := AddVNC(ip, port, "", "", s.Db)
	if err != nil {
		c.WriteMSG("vnc", err.Error())
	} else {
		c.WriteMSG("vnc", "Added VNC successfully!")
	}
	return err
}

func (s *WSServ) ServeWS(ws *websocket.Conn) {
	client := NewClient(ws)
	log.Println("Conn", client.Ip)
	s.Clients[client] = struct{}{}
	ticker := time.NewTicker(CFGClientPing)
	done := make(chan struct{})
	defer func() {
		log.Println("Disc", client.Ip)
		ticker.Stop()
		done <- struct{}{}
		client.Ws.Close()
		close(client.StatusChan)
		close(client.StartChan)
		close(client.StopChan)
		delete(s.Clients, client)
	}()

	if s.Started {
		go sendStart(client)
	}

	go func() {
		for {
			select {
			case <-done: return
			case <-ticker.C: client.WriteMSG("ping")
			}
		}
	}()

	for {
		client.Ws.SetDeadline(time.Now().Add(CFGClientTimeout))
		msg, err := client.ReadMSG()
		if err != nil {
			break
		}
		if len(msg) < 1 {
			continue
		}
		log.Printf("Got %s from %s", msg, client.Ip)

		switch msg[0] {
		case "status": client.StatusChan <- msg[1]
		case "start": client.StartChan <- msg[1]
		case "stop": client.StopChan <- msg[1]
		case "range": s.SendRange(client)
		case "vnc": go s.SendVNC(msg[1], msg[2], client)
		}
	}
}

func getIP(r *http.Request) string {
	ip := r.Header.Get("X-REAL-IP")
	if ip != "" {
		return ip
	}
	return r.RemoteAddr
}
