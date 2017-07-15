package server

import (
	"errors"
	"net/http"
	"sync"

	uuid "github.com/satori/go.uuid"

	"golang.org/x/net/websocket"
)

type (
	// OnClientRemoved will be called when a client is disconnected from the server.
	OnClientRemoved func(id string)
	// OnDispatch will be called when a message from a client is to be dispatched.
	// The original message will be provided along with the ID of the sender.
	// The function should return a slice of ClientIDs to broadcast the messages to,
	// as well as a possibly altered message to broadcast. If the slice of ClientIDs
	// is nil, the message will be broadcast to all connected clients.
	OnDispatch func(msg []byte, senderID string) ([]string, []byte)
	// OnErr will be called when the server encounters an error.
	OnErr func(error)
	// OnNewClient is called when a new client is connecting. The ClientID and Request
	// are provided so external bookkeeping can be done with the clients.
	// The function should return true if it's OK to connect the client, otherwise
	// the connection will be closed.
	OnNewClient func(id string, r *http.Request) bool

	// Server is the base type of this library. It does the low-level bookkeeping
	// of clients and dispatches messages and errors.
	Server struct {
		done            chan bool
		onClientRemoved OnClientRemoved
		onDispatch      OnDispatch
		onErr           OnErr
		onNewClient     OnNewClient
		mu              *sync.RWMutex
		messengers      map[string]*messenger
		running         bool
	}
)

var (
	// ErrClientNotFound signifies that a message was attempted to be dispatched
	// to a client that is no longer (or was never) connected.
	ErrClientNotFound = errors.New("monkey: client not found for broadcast")
	// ErrNilConn signifies that a client tried to connect, but the connection
	// received by the server was nil.
	ErrNilConn = errors.New("monkey: nil connection")
	// ErrNonServerConn signifies that the server received a connection from the client,
	// but for some reason it was not a server connection.
	ErrNonServerConn = errors.New("monkey: not a server connection")
)

// New returns an instantiated server.
func New(ocr OnClientRemoved, od OnDispatch, oe OnErr, onc OnNewClient) *Server {
	return &Server{
		onClientRemoved: ocr,
		onDispatch:      od,
		onErr:           oe,
		onNewClient:     onc,
		mu:              &sync.RWMutex{},
		messengers:      make(map[string]*messenger),
		running:         false,
	}
}

func (s *Server) add(m *messenger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messengers[m.id()] = m
}

// Dispatch calls the OnDispatch function to get the message to dispatch and
// the IDs of the clients to dispatch it to. It then sends the message to all
// the clients found.
func (s *Server) dispatch(msg []byte, senderID string) {
	ids, msg := s.onDispatch(msg, senderID)
	if (ids != nil && len(ids) == 0) || len(msg) == 0 {
		return
	}

	var sendTo []*messenger
	s.mu.RLock()
	for _, m := range s.messengers {
		var c *messenger
		if ids != nil {
			for _, id := range ids {
				if m.clientID == id {
					c = m
					break
				}
			}
		} else {
			c = m
		}
		if c == nil {
			// Since the server has no knowledge of the OnErr call expense, we
			// currently won't risk calling it while holding a lock.
			// s.err(ErrClientNotFound)
			continue
		}
		sendTo = append(sendTo, c)
		if len(sendTo) == len(ids) {
			break
		}
	}
	s.mu.RUnlock()

	for _, m := range sendTo {
		m.send(msg)
	}
}

// err passes the error to the server's OnError function.
func (s *Server) err(err error) {
	s.onErr(err)
}

// The handler used for when a client connects to the server.
func (s *Server) newClientHandler(conn *websocket.Conn) {
	if !s.running {
		return
	} else if conn == nil {
		s.err(ErrNilConn)
		return
	} else if !conn.IsServerConn() {
		s.err(ErrNonServerConn)
		return
	}

	id := uuid.NewV4().String()
	if !s.onNewClient(id, conn.Request()) {
		if err := conn.Close(); err != nil {
			s.err(err)
		}
		return
	}

	m := newMessenger(id, conn, s)
	s.add(m)
	m.start()
}

// Remove the client from the server.
func (s *Server) Remove(id string) {
	s.mu.Lock()
	m := s.messengers[id]
	s.remove(m, id)
	s.mu.Unlock()
	s.onClientRemoved(id)
}

func (s *Server) remove(m *messenger, id string) {
	if m == nil {
		return
	}
	m.done()
	delete(s.messengers, id)
}

// Start starts the server.
func (s *Server) Start(socketPattern string) {
	s.running = true
	http.Handle(socketPattern, websocket.Handler(s.newClientHandler))
}

// Stop serving new clients and close connections with all existing clients.
func (s *Server) Stop() {
	s.running = false

	s.mu.Lock()
	for id, m := range s.messengers {
		s.remove(m, id)
		// It doesn't really matter how expensive this will be since the server
		// is no longer running.
		s.onClientRemoved(id)
	}
	s.mu.Unlock()
}
