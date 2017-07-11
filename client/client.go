package client

import (
	"errors"
	"fmt"
	"io"
	"time"

	"golang.org/x/net/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type (
	Messenger struct {
		clientID string
		conn     *websocket.Conn
		send     chan []byte
		done     chan bool
		server   Server
	}

	Server interface {
		Broadcast(msg []byte)
		Remove(m *Messenger)
		Err(err error)
		IsRunning() bool
	}
)

func New(conn *websocket.Conn, server Server, clientID string) (*Messenger, error) {
	if conn == nil {
		return nil, errors.New("cannot create messenger with nil connection")
	} else if !server.IsRunning() {
		return nil, errors.New("cannot create messenger when server is not running")
	} else if !conn.IsServerConn() {
		return nil, errors.New("not a server connection when creating messenger")
	}

	c := &Messenger{
		clientID: clientID,
		conn:     conn,
		send:     make(chan []byte),
		done:     make(chan bool),
		server:   server,
	}
	return c, nil
}

func (m *Messenger) Start() {
	go m.startReader()
	go m.startWriter()
}

func (m *Messenger) Done() {
	m.done <- true
}

func (m *Messenger) startWriter() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		m.conn.Close()
	}()
	for {
		select {
		case msg := <-m.send:
			m.conn.SetWriteDeadline(time.Now().UTC().Add(writeWait))

			if err := websocket.Message.Send(m.conn, msg); err != nil {
				m.server.Err(err)
				return
			}
		case <-ticker.C:
			m.conn.SetWriteDeadline(time.Now().UTC().Add(writeWait))
			if err := websocket.Message.Send(m.conn, websocket.PingFrame); err != nil {
				m.server.Err(err)
				return
			}
		case <-m.done:
			return
		}
	}
}

func (m *Messenger) startReader() {
	defer func() {
		m.server.Remove(m)
		m.conn.Close()
	}()
	m.conn.SetReadDeadline(time.Now().UTC().Add(pongWait))
	for {
		select {
		case <-m.done:
			return
		default:
			var msg []byte
			if err := websocket.Message.Receive(m.conn, &msg); err == io.EOF {
				m.done <- true
			} else if err != nil {
				m.server.Err(err)
			} else {
				m.server.Broadcast(msg)
			}
		}
	}
}

func (m *Messenger) Send(msg []byte) {
	select {
	case m.send <- msg:
	default:
		m.server.Remove(m)
		err := fmt.Errorf("attempted to send message to disconnected client: %s", m.clientID)
		m.server.Err(err)
		m.done <- true
	}
}
