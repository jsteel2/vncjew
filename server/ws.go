package main

import (
	"errors"
	"log"
	"time"

	"golang.org/x/net/websocket"
)

type WSServ struct {
	Clients map[*websocket.Conn]bool
	Unregister chan *websocket.Conn
}

type Response struct {
	RemoteAddr string
	Response string
}

func NewWSServ() *WSServ {
	return &WSServ{
		Clients: make(map[*websocket.Conn]bool),
		Unregister: make(chan *websocket.Conn),
	}
}

func (s *WSServ) Get(str string, ws *websocket.Conn) ([]byte, error) {
	ws.SetWriteDeadline(time.Now().Add(CFGClientTimeout))
	_, err := ws.Write([]byte(str))
	if err != nil {
		return nil, err
	}
	ws.SetReadDeadline(time.Now().Add(CFGClientTimeout))
	msg := make([]byte, 1024)
	n, err := ws.Read(msg)
	return msg[:n], err
}

func (s *WSServ) Send(client, str string) (string, error) {
	for c := range s.Clients {
		if c.Request().RemoteAddr == client {
			msg, err := s.Get(str, c)
			if err != nil {
				s.Unregister <- c
				break
			}
			return string(msg), err
		}
	}
	return "", errors.New("Client does not exist")
}

func (s *WSServ) SendAll(str string) []Response {
	responses := make([]Response, 0, len(s.Clients))
	for c := range s.Clients {
		msg, err := s.Get(str, c)
		if err != nil {
			s.Unregister <- c
			continue
		}
		responses = append(responses, Response{
			RemoteAddr: c.Request().RemoteAddr,
			Response: string(msg),
		})
	}
	return responses
}

func (s *WSServ) ServeWS(ws *websocket.Conn) {
	log.Println("Conn", ws.Request().RemoteAddr)
	s.Clients[ws] = true
	defer func() {
		log.Println("Disc", ws.Request().RemoteAddr)
		delete(s.Clients, ws)
	}()
	for {
		select {
		case client := <-s.Unregister:
			if client == ws {
				return
			}
			s.Unregister <- client
		}
	}
}
