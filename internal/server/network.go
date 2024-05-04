package server

// TODO: implement ratelimiting.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lambdcalculus/scs/internal/client"
	"github.com/lambdcalculus/scs/internal/logger"
	"github.com/lambdcalculus/scs/pkg/packets"
)

func (srv *SCServer) listenTCP() {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%v", srv.config.PortTCP))
	if err != nil {
		srv.logger.Errorf("Couldn't listen on TCP (%v).", err)
		return
	}
	srv.logger.Infof("Listening TCP on port %v.", srv.config.PortTCP)
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			logger.Errorf("TCP listener error (%v).", err)
			break
		}
		c := client.NewTCPClient(conn, srv.logger)
		srv.logger.Debugf("New TCP connection from %v (IPID: %v).", c.Addr(), c.IPID())

		go srv.handleTCPClient(c)
	}
}

// Handles new raw TCP connections. Only used by legacy (AO) clients.
func (srv *SCServer) handleTCPClient(c *client.Client) {
	srv.clients.Add(c)
	defer srv.removeClient(c)

	// to this day, this is part of the handshake. lovely.
	c.WriteAO("decryptor", "DEPRECATED")
	for {
		p, err := c.ReadAO()
		if err != nil {
			srv.logger.Debugf("Error in connection from %v (IPID: %v): %s.", c.Addr(), c.IPID(), err)
		}
		if p == nil {
			if err == nil {
				srv.logger.Debugf("EOF reached in connection from %v (IPID: %v).", c.Addr(), c.IPID())
			}
			break
		}
		srv.logger.Tracef("Received message from %v (IPID: %v) via TCP: %#v", c.Addr(), c.IPID(), *p)
		go srv.handlePacketAO(c, *p)
	}
}

var (
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

func (srv *SCServer) listenWS() {
	mux := http.NewServeMux()
	mux.HandleFunc("/DATA", srv.dataEndpoint)
	mux.HandleFunc("/", srv.wsEndpoint)
	wsServer := &http.Server{
		Addr:           fmt.Sprintf(":%v", srv.config.PortWS),
		Handler:        mux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	// TODO: add a file server
	srv.logger.Infof("Listening WS on port %v.", srv.config.PortWS)
	srv.logger.Errorf("Stopped serving WS: %v.", wsServer.ListenAndServe())
}

// The handler for the '/' endpoint, for WebSocket connections to the server by
// both AO and SpriteChat.
func (srv *SCServer) wsEndpoint(w http.ResponseWriter, r *http.Request) {
	// TODO: set deadline for IO ops?
	// TODO: actually check the origin
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		srv.logger.Debugf("WS: (/) Couldn't upgrade connection from %v (%v).", r.RemoteAddr, err)
		return // bad request
	}
	client := client.NewWSClient(ws, srv.logger)
	srv.logger.Debugf("New WS connection from %v (IPID: %v).", r.RemoteAddr, client.IPID())

	go srv.handleWSClient(client)
}

// Handles a client after a successful websocket connection, first verifying it and
// then entering the read loop if it is successful. This client may be an AO or SpriteChat
// client.
func (srv *SCServer) handleWSClient(c *client.Client) {
	srv.clients.Add(c)
	defer srv.removeClient(c)
	if err := srv.validateClient(c); err != nil {
		srv.logger.Debugf("Couldn't determine client type from %v (IPID: %v) (%v). Disconnecting.", c.Addr(), c.IPID(), err)
		return
	}

	switch c.Type() {
	case client.AOClient:
		for {
			p, err := c.ReadAO()
			if err != nil {
				srv.logger.Debugf("Error in connection to %v (IPID: %v): %v.", c.Addr(), c.IPID(), err)
				return
			}
			srv.logger.Tracef("Received message from %v (IPID: %v) via WS: %#v", c.Addr(), c.IPID(), *p)
			go srv.handlePacketAO(c, *p)
		}
	case client.SCClient:
		for {
			p, err := c.ReadSC()
			if err != nil {
				if errors.Is(err, &json.SyntaxError{}) || errors.Is(err, &json.UnmarshalTypeError{}) {
					srv.logger.Debugf("Bad JSON by %v (IPID: %v) (%v).", c.Addr(), c.IPID(), err)
					continue
				}
				srv.logger.Debugf("Error in connection to %v (IPID: %v): %v.", c.Addr(), c.IPID(), err)
				break
			}
			srv.logger.Tracef("Received message from %v (IPID: %v) via WS: %#v", c.Addr(), c.IPID(), *p)
			go srv.handlePacketSC(c, *p)
		}
	}
}

// Validates a client as an AO or SC client.
// Returns an error if the type can't be identified.
func (srv *SCServer) validateClient(c *client.Client) error {
	// SC client sends 'hello' packet, while AO client waits for 'decryptor' packet.
	// So we wait a short time to see if we get a 'hello' packet - if not, we send a
	// 'decryptor' packet.
	b := make(chan []byte)
	e := make(chan error)
	go func(c *client.Client, b chan []byte, e chan error) {
		mesg, err := c.ReadWS()
		if err != nil {
			b <- nil
			e <- err
		}

		b <- mesg
		e <- nil
	}(c, b, e)

	timer := time.NewTimer(250 * time.Millisecond)
	var data []byte
	var err error
loop:
	for {
		select {
		case <-timer.C:
			// If the timer runs out, we see this packet to see if it's an AO client.
			c.WriteAO("decryptor", "DEPRECATED")
		case data = <-b:
			// Break out of the for loop when we receive data.
			err = <-e
			break loop
		}
	}

	if err != nil {
		return fmt.Errorf("Failed to read message (%v).", err)
	}

	if p := packets.MakeAOPacket(data); p.Header == "HI" {
		c.SetType(client.AOClient)
		srv.logger.Tracef("Received message from %v (IPID: %v) via WS: %s", c.Addr(), c.IPID(), data)
		go srv.handlePacketAO(c, p)
		return nil
	}

	p, err := packets.MakeSCPacket(data)
	if err == nil && p.Header == "hello" {
		c.SetType(client.SCClient)
		srv.logger.Tracef("Received message from %v (IPID: %v) via WS: %#v", c.Addr(), c.IPID(), p)
		go srv.handlePacketSC(c, p)
		return nil
	}
	return fmt.Errorf("Client is neither AO nor SC (%v).", err)
}

// Handles the '/DATA' endpoint used by the SpriteChat client. It sends the server
// data and disconnects.
func (srv *SCServer) dataEndpoint(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		srv.logger.Debugf("WS: (/DATA) Couldn't upgrade connection from %s (%v).", r.RemoteAddr, err)
		return // bad request
	}
	// TODO: I think the correct way to do this would be with a control message.
	defer ws.Close()

	reply := packets.PacketSC{
		Header: "SERVERHELLO",
		Data: packets.DataHelloServer{
			App:      "scs",
			Version:  "alpha",
			Name:     srv.config.Name,
			Desc:     srv.config.Desc,
			Players:  srv.clients.SizeJoined(),
			URL:      "",
			Packages: []string{},
		},
	}

	err = ws.WriteJSON(reply)
	if err != nil {
		srv.logger.Warnf("WS: (/DATA) Error writing JSON response (%v).", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	srv.logger.Debugf("WS: (/DATA) Sent data to %s.", r.RemoteAddr)
}
