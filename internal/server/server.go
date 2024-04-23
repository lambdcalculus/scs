// Package `server` handles client-server communication and the main server loop.
package server

// TODO: secure websockets

// TODO: abstract all (or almost all) outbound packets into methods from package `client`.

import (
	"fmt"

	"github.com/lambdcalculus/scs/internal/client"
	"github.com/lambdcalculus/scs/internal/config"
	"github.com/lambdcalculus/scs/internal/logger"
	"github.com/lambdcalculus/scs/internal/room"
	"github.com/lambdcalculus/scs/internal/uid"
	"github.com/lambdcalculus/scs/pkg/packets"
)

// TODO: consider getting rid of the struct and just having these as globals
type SCServer struct {
	config config.Server
	rooms  []*room.Room

	uidHeap uid.UIDHeap
	clients *client.List
	chars   config.CharList

	fatal chan error

	logger *logger.Logger
}

// Tries to create and prepare the server. May fail if configs are not set appropriately.
func MakeServer(log *logger.Logger) (*SCServer, error) {
	conf, err := config.ReadServer()
	if err != nil {
		return nil, fmt.Errorf("server: Couldn't configure server (%w).", err)
	}

	charsConf, err := config.ReadCharacters()
	if err != nil {
		return nil, fmt.Errorf("server: Couldn't read characters config (%w).", err)
	}
	log.Debugf("Characters config: %#v", charsConf)

	musicConf, err := config.ReadMusic()
	if err != nil {
		return nil, fmt.Errorf("server: Couldn't read music config (%w).", err)
	}
	log.Debugf("Music config: %#v", musicConf)

	rooms, err := room.MakeRooms(&charsConf, &musicConf)
	if err != nil {
		return nil, fmt.Errorf("server: Couldn't configure rooms (%w).", err)
	}

	srv := &SCServer{
		config:  conf,
		rooms:   rooms,
		uidHeap: *uid.CreateHeap(conf.MaxPlayers),
		clients: client.NewList(),
		fatal:   make(chan error),
		logger:  log,
	}
	srv.logger.Debugf("Successfully loaded server configuration: %#v", conf)
	return srv, nil
}

// Starts and runs the server.
func (srv *SCServer) Run() error {
	srv.logger.Info("Starting server.")
	if srv.config.PortWS > 0 {
		go srv.listenWS()
	}
	if srv.config.PortTCP > 0 {
		go srv.listenTCP()
	}
	select {
	case err := <-srv.fatal:
		return err
	}
}

// Looks for a client with the given UID. Returns `nil` if not found.
func (srv *SCServer) getByUID(id int) *client.Client {
	if id == uid.Unjoined {
		return nil
	}
	for c := range srv.clients.Clients() {
		if c.UID() == id {
			return c
		}
	}
	return nil
}

// Returns the room with the passed name. If there are none, returns `nil`.
func (srv *SCServer) getRoomByName(name string) *room.Room {
	for _, r := range srv.rooms {
		if name == r.Name() {
			return r
		}
	}
	return nil
}

// Returns the clients that are in the specified room.
func (srv *SCServer) getClientsInRoom(room *room.Room) []*client.Client {
    list := make([]*client.Client, 0, room.PlayerCount())
    for c := range srv.clients.Clients() {
        if c.Room() == room {
            list = append(list, c)
        }
    }
	return list
}

// Writes the specified packet to the specified room.
func (srv *SCServer) writeToRoomAO(r *room.Room, header string, contents ...string) {
    clients := srv.getClientsInRoom(r)
    for _, c := range clients {
        if c.Type() == client.AOClient {
            c.WriteAO(header, contents...)
        }
    }
}

// Sends an OOC message to all clients in the specified room.
func (srv *SCServer) sendOOCMessageToRoom(r *room.Room, username string, msg string, server bool) {
    clients := srv.getClientsInRoom(r)
    for _, c := range clients {
        c.SendOOCMessage(username, msg, server)
    }
}

// Sends a server message to all clients in the specified room.
func (srv *SCServer) sendServerMessageToRoom(r *room.Room, msg string) {
    clients := srv.getClientsInRoom(r)
    for _, c := range clients {
        c.SendOOCMessage(srv.config.Username, msg, true)
    }
}

// Disconnects and cleans up a client.
func (srv *SCServer) removeClient(c *client.Client) {
	if c.Room() != nil {
		c.Room().Leave(c.UID())
        c.SetRoom(nil)
	}
	if c.UID() != 0 {
		srv.uidHeap.Free(c.UID())
		srv.logger.Infof("Client with UID %v (IPID: %v) left.", c.UID(), c.IPID())
        c.SetUID(uid.Unjoined)
	}
	c.Disconnect()
	srv.clients.Remove(c)
	srv.sendRoomUpdateAllAO(packets.UpdatePlayer)
}

// Writes a message to all AO clients.
func (srv *SCServer) writeToAllAO(header string, contents ...string) {
	for c := range srv.clients.Clients() {
		if c.Type() == client.AOClient {
			c.WriteAO(header, contents...)
		}
	}
}

// Sends a server message to the client.
func (srv *SCServer) sendServerMessage(c *client.Client, msg string) {
	c.SendOOCMessage(srv.config.Username, msg, true)
}

// Sends an ARUP to all AO clients.
func (srv *SCServer) sendRoomUpdateAllAO(up packets.AreaUpdate) {
	// since we're doing the whole thing per client, this might be
	// really slow. we'll see if it matter. if it does, then TODO: make faster
	clients := srv.clients.ClientsJoined()
	for c := range clients {
		switch c.Type() {
		case client.AOClient:
			c.SendRoomUpdateAO(up)
		case client.SCClient:
			// TODO
		}
	}
}

// Attempts to move a client to room `dst`.
func (srv *SCServer) moveClient(c *client.Client, dst *room.Room) {
	currRoom := c.Room()
	if currRoom == dst {
		srv.sendServerMessage(c, "You are already in this room!")
		return
	}
    if (dst.LockState() & room.LockLocked != 0) && !dst.IsInvited(c.UID()) {
		srv.sendServerMessage(c, "You are not invited to this room!")
        return
    }

	srv.sendServerMessage(c, fmt.Sprintf("Moved to %v.", dst.Name()))
	srv.sendServerMessage(c, fmt.Sprintf("Description: %v", dst.Desc()))
	newCID, ok := dst.GetCIDByName(currRoom.GetNameByCID(c.CID()))
	if !ok {
		srv.sendServerMessage(c, "Your character is not in this room's list. Changing to Spectator.")
		newCID = room.SpectatorCID
	}
	if !dst.Enter(newCID, c.UID()) {
		srv.sendServerMessage(c, "Your character in this room is taken. Changing to Spectator.")
		newCID = room.SpectatorCID
		dst.Enter(newCID, c.UID())
	}
	currRoom.Leave(c.UID())

	c.SetRoom(dst)
    c.Update()
    c.ChangeChar(newCID)
    if c.Type() == client.AOClient {
        c.SendRoomUpdateAO(packets.UpdateAll & ^packets.UpdatePlayer)
    }
	// TODO: send only to adjacent rooms?
	srv.sendRoomUpdateAllAO(packets.UpdatePlayer)
	// TODO: enter/leave OOC message
}
