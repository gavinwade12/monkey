package server

import (
	"net/http"
	"sync"

	"golang.org/x/net/websocket"
)

type (
	MessengerFactory func(*websocket.Conn) Messenger

	Server struct {
		messengers *sync.Map
		factory    MessengerFactory
		err        chan error
		done       chan bool
	}

	Messenger interface {
		Send(msg []byte)
		Start()
		Done()
		ID() string
	}
)

func New() *Server {
	return &Server{
		messengers: &sync.Map{},
		err:        make(chan error),
		done:       make(chan bool),
	}
}

func (s *Server) Start(addr string) {
	onConnected := func(conn *websocket.Conn) {
		m := s.factory(conn)
		s.messengers.Store(m.ID(), m)
		m.Start()
	}

	http.Handle(addr, websocket.Handler(onConnected))
}
