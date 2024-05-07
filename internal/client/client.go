// Package `client` contains the client data structures and abstracts part of the AO/SC protocol.
package client

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/lambdcalculus/scs/internal/perms"
	"github.com/lambdcalculus/scs/internal/room"
	"github.com/lambdcalculus/scs/internal/uid"
	"github.com/lambdcalculus/scs/pkg/logger"
	"github.com/lambdcalculus/scs/pkg/packets"
)

// Defines whether the client is an AO or SpriteChat client.
type ClientType int

const (
	UndefClient ClientType = iota
	SCClient
	AOClient
)

// Defines in which situations a client is muted, through a bit mask.
type MuteState int

const (
	Unmuted MuteState = 0

	MutedIC MuteState = 1 << iota
	MutedOOC
	MutedMusic
	MutedJudge
	// TODO: add gimp/parrot
)

// Represents a client's connection and attributes.
type Client struct {
	mu sync.Mutex

	// connection data
	wsConn     *websocket.Conn
	tcpConn    net.Conn
	tcpScanner *bufio.Scanner
	addr       string
	clientType ClientType

	// identification data
	ident    string // the famed "HDID"
	ipid     string
	uid      int
	cid      int
	charname string // character name, i.e. the files the client is using
	perms    perms.Mask

	// state data
	showname   string
	username   string // OOC name
	charPicked bool   // a client is technically joined before picking a character, but to announce its entrance properly we need an extra variable. ugh.
	room       *room.Room
	side       string
	mute       MuteState
	autopass   bool // TODO: implement
	lastMsg    string

	// pair data
	pair PairData

	// logger
	logger *logger.Logger
}

type PairData struct {
	WantedCID  int
	LastChar   string
	LastEmote  string
	LastOffset string
	LastFlip   string
}

// Makes a new client over a TCP connection. The client will log to the specified logger.
func NewTCPClient(conn net.Conn, log *logger.Logger) *Client {
	ipid := hashIP(conn.RemoteAddr())
	client := &Client{
		tcpConn:    conn,
		addr:       conn.RemoteAddr().String(),
		clientType: AOClient,
		ipid:       ipid,
		uid:        uid.Unjoined,
		cid:        room.SpectatorCID,
		pair:       PairData{WantedCID: -1},
		logger:     log,
	}

    // The default maximum token size is 64KiB.
    // Way bigger than we need, but nobody's gonna crash the server if they send something that long, lol.
	scanner := bufio.NewScanner(conn)
	split := splitAt('%')
	scanner.Split(split)
	client.tcpScanner = scanner

	return client
}

// Makes a new client over a WebSocket connection. The client will log to the specified logger.
func NewWSClient(conn *websocket.Conn, log *logger.Logger) *Client {
    // Read limit is 64KiB, just because that's the default used by the scanner on the TCP side.
    // Can be changed later, if necessary.
    conn.SetReadLimit(64 << 10)

	ipid := hashIP(conn.RemoteAddr())
	return &Client{
		wsConn: conn,
		addr:   conn.RemoteAddr().String(),
		ipid:   ipid,
		uid:    uid.Unjoined,
		cid:    room.SpectatorCID,
		pair:   PairData{WantedCID: -1},
		logger: log,
	}
}

// Returns whether the client is connected via WebSocket.
func (c *Client) IsWS() bool {
	return c.wsConn != nil
}

// Reads a WebSocket message.
func (c *Client) ReadWS() ([]byte, error) {
	_, b, err := c.wsConn.ReadMessage()
	return b, err
}

// TODO: add checks to all the AO vs. SC funcs?

// Waits for the next message from the client and interprets it as an AO packet.
func (c *Client) ReadAO() (*packets.PacketAO, error) {
	if c.IsWS() {
		_, b, err := c.wsConn.ReadMessage()
		if err != nil {
			return nil, err
		}
		p := packets.MakeAOPacket(b)
		p.Decode()
		return &p, nil
	}
	if c.tcpScanner.Scan() {
		p := packets.MakeAOPacket(c.tcpScanner.Bytes())
		p.Decode()
		return &p, nil
	}
	return nil, c.tcpScanner.Err()
}

// Waits for the next message from the client and interprets it as a SpriteChat packet.
func (c *Client) ReadSC() (*packets.PacketSC, error) {
	var p packets.PacketSC
	err := c.wsConn.ReadJSON(&p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Creates and writes an encoded AO packet to the client.
func (c *Client) WriteAO(header string, contents ...string) {
	p := packets.PacketAO{
		Header:   header,
		Contents: contents,
	}
	c.WriteAOPacket(p)
}

// Writes an AO packet to the client.
func (c *Client) WriteAOPacket(pkt packets.PacketAO) {
	pkt.Encode()
	c.writef("%s#%s#%%", pkt.Header, strings.Join(pkt.Contents, "#"))
}

// Creates and writes a SC packet to the client.
func (c *Client) WriteSC(header string, data interface{}) {
	mesg := map[string]interface{}{
		"header": header,
		"data":   data,
	}
	if err := c.wsConn.WriteJSON(mesg); err != nil {
		c.logger.Tracef("Couldn't write JSON to %v (IPID: %v) (%v).", c.addr, c.ipid, err)
		return
	}
	b, _ := json.MarshalIndent(mesg, "", "  ") // cannot fail if we got here
	c.logger.Tracef("Sent to %v (IPID: %v) via WS: %s.\n", c.addr, c.ipid, b)
}

// Writes a SC packet to the client.
func (c *Client) WriteSCPacket(pkt packets.PacketSC) {
	c.WriteSC(pkt.Header, pkt.Data)
}

// Disconnects the client.
func (c *Client) Disconnect() {
	if c.tcpConn != nil {
		c.logger.Debugf("%v (IPID: %v) disconnected (TCP).", c.addr, c.ipid)
		c.tcpConn.Close()
	}
	if c.wsConn != nil {
		// TODO: deal with close types
		c.logger.Debugf("%v (IPID: %v) disconnected (WS).", c.addr, c.ipid)
		c.wsConn.Close()
	}
}

// Sends an OOC message to the client.
func (c *Client) SendOOCMessage(name string, msg string, server bool) {
	var s string
	if server {
		s = "1"
	} else {
		s = "0"
	}

	switch c.Type() {
	case AOClient:
		c.WriteAO("CT", name, msg, s)
	case SCClient:
		// TODO
	}
}

// Attempts a character change to the passed CID.
func (c *Client) ChangeChar(cid int) {
	if !c.Room().ChangeChar(c.uid, cid) {
		c.Room().LogEvent(room.EventFail, "%s failed to change characters to %s (%v).", c.LongString(),
			c.Room().GetNameByCID(cid), cid)
		return
	}
	if cid == c.CID() {
		return
	}

    charname := c.Room().GetNameByCID(cid)
    if !c.charPicked {
        c.Room().LogEvent(room.EventCharacter, "%s picked %s (%v).", c.LongString(), charname, cid)
    } else {
        c.Room().LogEvent(room.EventCharacter, "%s changed to %s (%v).", c.LongString(), charname, cid)
    }

	c.SetCID(cid)
	c.SetCharname(c.Room().GetNameByCID(c.CID()))
	switch c.clientType {
	case AOClient:
		c.WriteAO("PV", "OBSOLETE", "CID", strconv.Itoa(cid))
	case SCClient:
		// TODO
	}
}

// Sends the client a pop-up.
func (c *Client) Notify(msg string) {
	switch c.clientType {
	case AOClient:
		c.WriteAO("BB", msg)
	case SCClient:
		// TODO
	}
}

// Sends ARUPs to the client according to the input.
func (c *Client) SendRoomUpdateAO(up packets.AreaUpdate) {
	var players []string
	var statuses []string
	var cms []string
	var locks []string

	// We update this client's room, and all the adjacent ones.
	vis := c.Room().Visible()

	// Only allocate as necessary.
	if up&packets.UpdatePlayer != 0 {
		players = make([]string, len(vis))
	}
	if up&packets.UpdateStatus != 0 {
		statuses = make([]string, len(vis))
	}
	if up&packets.UpdateManager != 0 {
		cms = make([]string, len(vis))
	}
	if up&packets.UpdateLock != 0 {
		locks = make([]string, len(vis))
	}

	for i, r := range vis {
		// Branch prediction will optimize this for us, I hope.
		if up&packets.UpdatePlayer != 0 {
			players[i] = strconv.Itoa(r.PlayerCount())
		}
		if up&packets.UpdateStatus != 0 {
			statuses[i] = r.Status()
		}
		if up&packets.UpdateManager != 0 {
			// TODO: CMs
			cms[i] = "FREE"
		}
		if up&packets.UpdateLock != 0 {
			locks[i] = r.LockString()
		}
	}
	// TODO: spritechat
	if up&packets.UpdatePlayer != 0 {
		c.WriteAO("ARUP#0", players...)
	}
	if up&packets.UpdateStatus != 0 {
		c.WriteAO("ARUP#1", statuses...)
	}
	if up&packets.UpdateManager != 0 {
		c.WriteAO("ARUP#2", cms...)
	}
	if up&packets.UpdateLock != 0 {
		c.WriteAO("ARUP#3", locks...)
	}
}

// Notifies a client that it has been kicked, along with the reason.
// (Does NOT disconnect the client, use removeClient after.)
func (c *Client) NotifyKick(reason string) {
	switch c.clientType {
	case AOClient:
		c.WriteAO("KK", reason)
	case SCClient:
		// TODO
	}
}

// Adds the guard button on the client (AO-only?).
func (c *Client) AddGuard() {
	switch c.clientType {
	case AOClient:
		c.WriteAO("AUTH", "1")
	case SCClient:
		// no-op?
	}
}

// Sends the client a mod call packet.
func (c *Client) ModCall(msg string) {
	switch c.clientType {
	case AOClient:
		c.WriteAO("ZZ", msg)
	case SCClient:
		// TODO
	}
}

// Sends the client the char list of the room it is currently in.
func (c *Client) UpdateCharList() {
	switch c.Type() {
	case AOClient:
		c.WriteAO("SC", c.Room().Chars()...)
		c.WriteAO("CharsCheck", c.Room().TakenList()...)
	case SCClient:
		// TODO
	}
}

// Sends the client the song list of the room it is currently in.
func (c *Client) UpdateMusicList() {
	switch c.Type() {
	case AOClient:
		c.WriteAO("FM", c.Room().MusicList()...)
	case SCClient:
		// TODO
	}
}

// Sends the client the list of visible rooms from the room it is currently in.
func (c *Client) UpdateRoomList() {
	switch c.Type() {
	case AOClient:
		c.WriteAO("FA", c.Room().VisibleNames()...)
	case SCClient:
		// TODO
	}
}

// Updates the background according to the current room.
func (c *Client) UpdateBackground() {
	switch c.Type() {
	case AOClient:
		c.WriteAO("BN", c.Room().Background())
	case SCClient:
		// TODO
	}
}

// Updates the side list in the client's dropdown.
func (c *Client) UpdateSides() {
	switch c.Type() {
	case AOClient:
		c.WriteAO("SD", strings.Join(c.Room().Sides(), "*"))
	case SCClient:
		// TODO
	}
}

// Updates the prosecution/def bars.
func (c *Client) UpdateBars() {
    switch c.Type() {
    case AOClient:
        c.WriteAO("HP", "1", strconv.Itoa(int(c.Room().Bar(packets.BarDef))))
        c.WriteAO("HP", "2", strconv.Itoa(int(c.Room().Bar(packets.BarPro))))
    case SCClient:
        // TODO
    }
}

// Updates the music according to the current room.
func (c *Client) UpdateSong() {
	switch c.Type() {
	case AOClient:
		// TODO: using the spectator CID makes it so no message is displayed.
		// this might not be the best thing, we e.g. say the room itself plays the song, etc.
		c.WriteAO("MC", c.Room().Song(), // Song name.
			strconv.Itoa(room.SpectatorCID), // CID.
			c.Room().Name(),                 // Showname. We're using the room's name.
			"1",                             // Loop
			"0",                             // Channel 0 (default for BGM).
			strconv.Itoa(int(packets.EffectFadeIn|packets.EffectFadeOut))) // Fade in and fade out.
	case SCClient:
		// TODO
	}
}

// Updates the ambiance according to the current room.
func (c *Client) UpdateAmbiance() {
	switch c.Type() {
	case AOClient:
		// We send this as though the room itself has played the song.
		c.WriteAO("MC", c.Room().Ambiance(), // Song name.
			strconv.Itoa(room.SpectatorCID), // CID. Will be ignored by 2.6+ since we give the showname.
			c.Room().Name(),                 // Showname. We're using the room's name.
			"1",                             // Loop
			"1",                             // Channel 1 (default for Ambiance).
			strconv.Itoa(int(packets.EffectFadeIn|packets.EffectFadeOut))) // Fade in and fade out.
	case SCClient:
		// TODO
	}
}

// Updates room list, char list, music list, background, sides, current song, and ambiance,
// all according to the current room the client is in.
func (c *Client) Update() {
	c.UpdateRoomList()
	c.UpdateMusicList()
	c.UpdateCharList()
	c.UpdateBackground()
	c.UpdateSides()
    c.UpdateBars()
	c.UpdateSong()
	c.UpdateAmbiance()
}

// Returns a string that helps identify the client. Used in log messages or commands like
// `/get room`. This one does not show the IPID, use [Client.LongString] for that.
// The format is: `[{UID}] "{username}" as {charname} ({CID})`.
// If the username is unset, it won't be shown.
// If the showname isn't set, charname is used.
func (c *Client) String() string {
	var user string
	if c.Username() != "" {
		user = fmt.Sprintf("\"%s\" as ", c.Username())
	}
	return fmt.Sprintf("[%v] %s%s (%v)", c.UID(), user, c.Charname(), c.CID())
}

// Like [Client.String], but with the IPID. Should be used where only moderators can see.
// Format: `[{UID}] "{username}" as "{charname}" ({CID}) IPID: {IPID}`.
func (c *Client) LongString() string {
	return c.String() + fmt.Sprintf(" IPID: %v", c.IPID())
}

// Like [Client.String] but only has UID, charname and username.
// Format: `[{UID}] {charname} ({username})`.
func (c *Client) ShortString() string {
	var user string
	if c.Username() != "" {
		user = fmt.Sprintf(" (%s)", c.Username())
	}
	return fmt.Sprintf("[%v] %s%s", c.UID(), c.Charname(), user)
}

// Checks whether a client has joined the server.
func (c *Client) Joined() bool {
	// TODO: have a client only be 'joined' when it has picked a character?
	return c.UID() != uid.Unjoined
}

// Returns whether a client is iniswapping.
func (c *Client) Iniswapping() bool {
	return c.Charname() != c.Room().GetNameByCID(c.CID())
}

// Returns whether the client satisfies the passed permission mask.
func (c *Client) HasPerms(p perms.Mask) bool {
	return c.Perms()&p == p
}

func (c *Client) Addr() string {
	if c.wsConn != nil {
		return c.wsConn.RemoteAddr().String()
	}
	return c.tcpConn.RemoteAddr().String()
}

func (c *Client) Type() ClientType {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.clientType
}

func (c *Client) SetType(t ClientType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clientType = t
}

func (c *Client) IPID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ipid
}

func (c *Client) UID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.uid
}

func (c *Client) SetUID(uid int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.uid = uid
}

func (c *Client) CID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cid
}

func (c *Client) SetCID(cid int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cid = cid
}

func (c *Client) Charname() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.charname
}

func (c *Client) SetCharname(char string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.charname = char
}

func (c *Client) Perms() perms.Mask {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.perms
}

func (c *Client) SetPerms(p perms.Mask) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.perms = p
}

func (c *Client) Room() *room.Room {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.room
}

func (c *Client) SetRoom(r *room.Room) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.room = r
}

func (c *Client) Ident() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ident
}

func (c *Client) SetIdent(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ident = id
}

func (c *Client) Showname() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.showname
}

func (c *Client) SetShowname(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.showname = name
}

func (c *Client) Username() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.username
}

func (c *Client) SetUsername(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.username = name
}

func (c *Client) CharPicked() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.charPicked
}

func (c *Client) SetCharPicked(b bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.charPicked = b
}

func (c *Client) Side() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.side
}

func (c *Client) SetSide(side string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.side = side
}

func (c *Client) MuteState() MuteState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mute
}

func (c *Client) SetMute(m MuteState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mute = m
}

func (c *Client) AddMute(m MuteState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mute |= m
}

func (c *Client) RemoveMute(m MuteState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mute &= ^m
}

func (c *Client) LastMsg() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastMsg
}

func (c *Client) SetLastMsg(msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastMsg = msg
}

func (c *Client) PairData() PairData {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pair
}

func (c *Client) SetPairData(pd PairData) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pair = pd
}

func (c *Client) write(mesg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.wsConn == nil {
		if _, err := fmt.Fprint(c.tcpConn, mesg); err != nil {
			c.logger.Debugf("Failed to write message to %v (IPID: %v) via TCP (%v). Message: %s.", c.addr, c.ipid, err, mesg)
			return
		}
		c.logger.Tracef("Sent message to %v (IPID: %v) via TCP: %s", c.addr, c.ipid, mesg)
		return
	}

	w, err := c.wsConn.NextWriter(websocket.TextMessage)
	if err != nil {
		c.logger.Debugf("Failed to write message to %v (IPID: %v) via WS (%v). Message: %s.", c.addr, c.ipid, err, mesg)
		return
	}
	defer w.Close()

	if _, err := fmt.Fprint(w, mesg); err != nil {
		c.logger.Debugf("Failed to write message to %v (IPID: %v) via WS (%v). Message: %s.", c.addr, c.ipid, err, mesg)
		return
	}
	c.logger.Tracef("Sent message to %v (IPID: %v) via WS: %s", c.addr, c.ipid, mesg)
}

func (c *Client) writef(format string, args ...any) {
	c.write(fmt.Sprintf(format, args...))
}
