package server

import (
	"io"
	"time"

	"golang.org/x/net/websocket"
)

const (
	maxMessageSize = 512
	// pingPeriod is the amount of time to wait before pinging the client
	// to keep the connection alive. It must be smaller than pongWait.
	pingPeriod = (pongWait * 9) / 10
	// pongWait is used as the readDeadline
	pongWait = 60 * time.Second
	// writeWait is used as the writeDeadline
	writeWait = 10 * time.Second
)

type (
	// messenger resembles a client connected to the server.
	messenger struct {
		clientID string
		conn     *websocket.Conn
		doneCh   chan bool
		sendCh   chan []byte
		server   *Server
	}
)

func newMessenger(clientID string, conn *websocket.Conn, server *Server) *messenger {
	return &messenger{
		clientID: clientID,
		conn:     conn,
		doneCh:   make(chan bool),
		sendCh:   make(chan []byte),
		server:   server,
	}
}

// done closes the connection and stops the reader and writer.
func (m *messenger) done() {
	if err := m.conn.Close(); err != nil {
		m.server.err(err)
	}

	// send two to end reader and writer
	m.doneCh <- true
	m.doneCh <- true
}

func (m *messenger) id() string {
	return m.clientID
}

// send a message to the client.
func (m *messenger) send(msg []byte) {
	select {
	case m.sendCh <- msg:
	default:
		m.server.Remove(m.clientID)
		m.server.err(ErrClientNotFound)
		m.doneCh <- true
	}
}

// start the messenger
func (m *messenger) start() {
	go m.startReader()
	go m.startWriter()
}

// Start a goroutine to do all the reading from the messenger's connection. This guarantees
// that there will only be one reader per connection.
func (m *messenger) startReader() {
	defer func() {
		m.server.Remove(m.clientID)
		m.conn.Close()
	}()
	m.conn.SetReadDeadline(time.Now().UTC().Add(pongWait))
	for {
		select {
		case <-m.doneCh:
			return
		default:
			var msg []byte
			if err := websocket.Message.Receive(m.conn, &msg); err == io.EOF {
				m.doneCh <- true
			} else if err != nil {
				m.server.err(err)
			} else {
				m.server.dispatch(msg, m.clientID)
			}
		}
	}
}

// Start a goroutine to do all the writing to the messenger's connection. This guarantees
// that there will only be one writer per connection.
func (m *messenger) startWriter() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		m.conn.Close()
	}()
	for {
		select {
		case msg := <-m.sendCh:
			m.conn.SetWriteDeadline(time.Now().UTC().Add(writeWait))

			if err := websocket.Message.Send(m.conn, msg); err != nil {
				m.server.err(err)
				return
			}
		case <-ticker.C:
			m.conn.SetWriteDeadline(time.Now().UTC().Add(writeWait))
			if err := websocket.Message.Send(m.conn, websocket.PingFrame); err != nil {
				m.server.err(err)
				return
			}
		case <-m.doneCh:
			return
		}
	}
}
