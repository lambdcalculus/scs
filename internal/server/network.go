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
	"github.com/lambdcalculus/scs/pkg/logger"
	"github.com/lambdcalculus/scs/pkg/packets"
)

var (
    // The upgrader for WebSocket connections.
	// TODO: set deadline for IO ops?
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
        // TODO: actually check the origin
        CheckOrigin: func(r *http.Request) bool { return true },
	}
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


func (srv *SCServer) listenWS() {
	mux := http.NewServeMux()
    mux.HandleFunc("/", srv.rootEndpoint)
    mux.HandleFunc("/GAME", srv.gameEndpoint)
	mux.HandleFunc("/DATA", srv.dataEndpoint)
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

// The handler for the '/GAME' endpoint, for WebSocket connections to the server by SC.
func (srv *SCServer) gameEndpoint(w http.ResponseWriter, r * http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		srv.logger.Debugf("WS: (/) Couldn't upgrade connection from %v (%v).", r.RemoteAddr, err)
		return // bad request
	}
	client := client.NewWSClient(ws, client.SCClient, srv.logger)
	srv.logger.Debugf("New WS connection from %v (IPID: %v).", r.RemoteAddr, client.IPID())

	go srv.handleWSClient(client)
}

// The handler for the '/' endpoint, for WebSocket connections to the server by AO.
func (srv *SCServer) rootEndpoint(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		srv.logger.Debugf("WS: (/) Couldn't upgrade connection from %v (%v).", r.RemoteAddr, err)
		return // bad request
	}
	client := client.NewWSClient(ws, client.AOClient, srv.logger)
	srv.logger.Debugf("New WS connection from %v (IPID: %v).", r.RemoteAddr, client.IPID())

    // The AO client still expects us to send this.
    client.WriteAO("decryptor", "DEPRECATED")

	go srv.handleWSClient(client)
}

// Handles a client after a successful websocket connection, first verifying it and
// then entering the read loop if it is successful. This client may be an AO or SpriteChat
// client.
func (srv *SCServer) handleWSClient(c *client.Client) {
	srv.clients.Add(c)
	defer srv.removeClient(c)

	switch c.Type() {
	case client.AOClient:
		for {
			p, err := c.ReadAO()
			if err != nil {
				srv.logger.Debugf("Error in connection to %v (IPID: %v): %v.", c.Addr(), c.IPID(), err)
				return
			}
			srv.logger.Tracef("Received message from %v (IPID: %v) via WS: %#v", c.Addr(), c.IPID(), *p)
			// go srv.handlePacketAO(c, *p)
            // TODO: handle packets with a queue? they should be read in order, but packet handling shouldn't
            // cease the reading. this seems fine though
            srv.handlePacketAO(c, *p)
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
