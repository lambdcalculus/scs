// Package `server` handles client-server communication and the main server loop.
package server

// TODO: secure websockets

// TODO: abstract all (or almost all) outbound packets into methods from package `client`.

import (
	"fmt"

	"github.com/lambdcalculus/scs/internal/client"
	"github.com/lambdcalculus/scs/internal/config"
	"github.com/lambdcalculus/scs/internal/db"
	"github.com/lambdcalculus/scs/internal/perms"
	"github.com/lambdcalculus/scs/internal/room"
	"github.com/lambdcalculus/scs/internal/uid"
	"github.com/lambdcalculus/scs/pkg/logger"
	"github.com/lambdcalculus/scs/pkg/packets"
)

type SCServer struct {
	config *config.Server
	db     *db.Database

	roles   []perms.Role
	rooms   []*room.Room
	mgrRole perms.Role // role used for /manage

	uidHeap uid.UIDHeap
	clients *client.List

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

	roomsConf, err := config.ReadRooms()
	if err != nil {
		return nil, fmt.Errorf("server: Couldn't read rooms config (%w).", err)
	}
	rooms, err := room.MakeRooms(roomsConf, charsConf, musicConf)
	if err != nil {
		return nil, fmt.Errorf("server: Couldn't configure rooms (%w).", err)
	}

	rolesConf, err := config.ReadRoles()
	if err != nil {
		return nil, fmt.Errorf("server: Couldn't read roles config (%w)", err)
	}
	log.Debugf("Roles config: %#v", rolesConf)
	roles, err := perms.MakeRoles(rolesConf)
	if err != nil {
		return nil, fmt.Errorf("server: Couldn't configure roles (%w).", err)
	}

	execDir, err := config.ExecDir()
	if err != nil {
		return nil, fmt.Errorf("server: Couldn't get executable directory (%w).", err)
	}
	db, err := db.Init(execDir + "/database.sqlite")
	if err != nil {
		return nil, fmt.Errorf("server: Couldn't initialize database (%w).", err)
	}

	// Find manager role.
	var mgrRole perms.Role
	found := false
	for _, r := range roles {
		if r.Name == conf.ManagerRole {
			found = true
			mgrRole = r
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("server: Manager role '%s' not in roles list.", conf.ManagerRole)
	}

	srv := &SCServer{
		config:  conf,
		db:      db,
		roles:   roles,
		rooms:   rooms,
		mgrRole: mgrRole,
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
	// TODO: don't panic if one of the listeners panics
	if srv.config.PortWS > 0 {
		go srv.listenWS()
	}
	if srv.config.PortTCP > 0 {
		go srv.listenTCP()
	}
	if srv.config.PortRPC > 0 {
		go srv.listenRPC()
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

// Looks for all clients with the given IPID. If none found, returns `nil`.
func (srv *SCServer) getByIPID(id string) []*client.Client {
	var clients []*client.Client
	for c := range srv.clients.Clients() {
		if c.IPID() == id {
			clients = append(clients, c)
		}
	}
	return clients
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
func (srv *SCServer) sendServerMessageToRoom(r *room.Room, format string, a ...any) {
	r.LogEvent(room.EventServerMsg, fmt.Sprintf("%s: %s", srv.config.Username, fmt.Sprintf(format, a...)))
	srv.sendOOCMessageToRoom(r, srv.config.Username, fmt.Sprintf(format, a...), true)
}

func (srv *SCServer) kickClient(c *client.Client, reason string) {
	c.NotifyKick(reason)
	srv.removeClient(c)
}

// Disconnects and cleans up a client.
func (srv *SCServer) removeClient(c *client.Client) {
	if c.Room() != nil {
		srv.moveClient(c, nil)
		// Don't send disconnect message if someone only got to the character list.
		if c.CharPicked() {
			srv.sendServerMessageToRoom(c.Room(), fmt.Sprintf("%s has disconnected.", c.ShortString()))
		}
	}
	if c.UID() != uid.Unjoined {
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
func (srv *SCServer) sendServerMessage(c *client.Client, format string, a ...any) {
	c.SendOOCMessage(srv.config.Username, fmt.Sprintf(format, a...), true)
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

// Attempts to move a client to room `dst`. `dst` can be `nil`, to be used when disconnecting a client.
func (srv *SCServer) moveClient(c *client.Client, dst *room.Room) {
	currRoom := c.Room()
	if currRoom == dst {
		srv.sendServerMessage(c, "You are already in this room!")
		return
	}

	// remove manager privileges
	if currRoom.IsManager(c.UID()) {
		currRoom.RemoveManager(c.UID())
		c.RemoveRole(srv.mgrRole)
		srv.sendServerMessageToRoom(currRoom, "%s is no longer managing this room.", c.ShortString())
	}

	// only used when disconnecting
	if dst == nil {
		currRoom.LogEvent(room.EventExit, "%s disconnected.", c.LongString())
		currRoom.Leave(c.UID())
		c.SetRoom(nil)
		return
	}

	// check invite
	if (dst.LockState()&room.LockLocked != 0) && !dst.IsInvited(c.UID()) {
		dst.LogEvent(room.EventFail, "%s tried to enter uninvited.", c.LongString())
		srv.sendServerMessage(c, "You are not invited to this room!")
		return
	}

	srv.sendServerMessage(c, "Moved to [%v] %s. Description: %s", dst.ID(), dst.Name(), dst.Desc())

	// check character
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

	// TODO: autopass on/off or sneaking? see how other servers do it
	srv.sendServerMessageToRoom(dst, "%s enters from [%v] %s.", c.ShortString(), currRoom.ID(), currRoom.Name())
	dst.LogEvent(room.EventEnter, "%s enters from [%v] %s.", c.LongString(), currRoom.ID(), currRoom.Name())
	c.SetRoom(dst)

	currRoom.Leave(c.UID())
	srv.sendServerMessageToRoom(currRoom, "%s leaves to [%v] %s.", c.ShortString(), dst.ID(), dst.Name())
	currRoom.LogEvent(room.EventExit, "%s leaves to [%v] %s.", c.LongString(), dst.ID(), dst.Name())

	c.Update()
	c.ChangeChar(newCID)

	if c.Type() == client.AOClient {
		c.SendRoomUpdateAO(packets.UpdateAll & ^packets.UpdatePlayer)
	} // TODO: add spritechat

	// TODO: send only to adjacent rooms?
	srv.sendRoomUpdateAllAO(packets.UpdatePlayer)
}
