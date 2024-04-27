package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lambdcalculus/scs/internal/client"
	"github.com/lambdcalculus/scs/internal/logger"
	"github.com/lambdcalculus/scs/internal/room"
	"github.com/lambdcalculus/scs/pkg/packets"
)

type handleFuncAO func(srv *SCServer, c *client.Client, contents []string)

type handlerAO struct {
	handleFunc handleFuncAO
	minArgs    int
	maxArgs    int
	needJoined bool
}

var handlerMapAO = map[string]handlerAO{
	"HI":      {(*SCServer).handleHI, 1, 1, false},
	"ID":      {(*SCServer).handleID, 2, 2, false},
	"askchaa": {(*SCServer).handleAskCounts, 0, 0, false},
	"RC":      {(*SCServer).handleRequestChars, 0, 0, false},
	"RM":      {(*SCServer).handleRequestMusic, 0, 0, false},
	"RD":      {(*SCServer).handleDone, 0, 0, false},
	"CC":      {(*SCServer).handleChangeChars, 3, 3, true},
	"CT":      {(*SCServer).handleOOC, 2, 2, true},
	"MC":      {(*SCServer).handleMusicArea, 2, 4, true},
	"CH":      {(*SCServer).handleCheck, 1, 1, true},
	"MS":      {(*SCServer).handleIC, 15, 26, true},
	// TODO:
	// HP (judge bars)
	// RT (wt/ce and testimony)
	// AUTH (authentication)
	// ZZ (call mod)

	// These will be repurposed for a better inventory system.
	// LE (evidence list)
	// PE (add evidence)
	// DE (remove evidence)
	// EE (edit evidence)

	// Who even uses this? I'll probably not implement it.
	// SETCASE (case preferences)
	// CASEA (case alert)
}

func (srv *SCServer) handlePacketAO(c *client.Client, pkt packets.PacketAO) {
	if handler, ok := handlerMapAO[pkt.Header]; ok {
		l := len(pkt.Contents)
		if l < handler.minArgs || l > handler.maxArgs {
			srv.logger.Infof("Bad '%v' packet from %v (IPID: %v): %#v", pkt.Header, c.Addr(), c.IPID(), pkt)
			return
		}
		if !c.Joined() && handler.needJoined {
			srv.logger.Infof("'%v' packet from %v (IPID: %v) but isn't joined: %#v", pkt.Header, c.Addr(), c.IPID(), pkt)
			return
		}
		handler.handleFunc(srv, c, pkt.Contents)
	}
}

func (srv *SCServer) handleHI(c *client.Client, contents []string) {
	c.SetIdent(contents[0])
	c.WriteAO("ID", "scs", "0")
	c.WriteAO("PN", strconv.Itoa(srv.clients.SizeJoined()), strconv.Itoa(srv.config.MaxPlayers))

	c.WriteAO("FL",
		"yellowtext", "flipping", "customobjections", "fastloading", "noencryption", // 2.1.0 features
		"deskmod", // "evidence",                                                       // 2.3 - 2.5 features
		"cccc_ic_support", "arup", //"casing_alerts", "modcall_reason",                // 2.6 features
		"looping_sfx", "additive", "effects",                                        // 2.8 features
		"y_offset", "expanded_desk_mods",                                            // 2.9 features
		// "auth_packet",                                                               // 2.9.1 feature
	)

	if srv.config.AssetURL != "" {
		c.WriteAO("ASS", srv.config.AssetURL)
	}
}

func (srv *SCServer) handleID(c *client.Client, contents []string) {
	// no-op
}

func (srv *SCServer) handleAskCounts(c *client.Client, contents []string) {
	charCount := strconv.Itoa(srv.rooms[0].CharsLen())
	musicCount := strconv.Itoa(srv.rooms[0].MusicLen())

	if srv.clients.SizeJoined() >= srv.config.MaxPlayers {
		c.Notify("Server is full.")
		logger.Info("A client couldn't join because of the server is full.")
		srv.removeClient(c)
		return
	}
	// TODO: implement evidence
	c.WriteAO("SI", charCount, "0", musicCount)
}

func (srv *SCServer) handleRequestChars(c *client.Client, contents []string) {
	c.WriteAO("SC", srv.rooms[0].Chars()...)
	c.WriteAO("CharsCheck", srv.rooms[0].TakenList()...)
}

func (srv *SCServer) handleRequestMusic(c *client.Client, contents []string) {
	// TODO: Maybe better have everything pre-prepared. But I doubt this is too slow to matter.

	// AO uses this for both areas and songs.
	vis := srv.rooms[0].VisibleNames()
	music := srv.rooms[0].MusicList()

	list := make([]string, 0, len(vis)+len(music))
	list = append(list, vis...)
	list = append(list, music...)
	c.WriteAO("SM", list...)
}

func (srv *SCServer) handleDone(c *client.Client, contents []string) {
	// Client has committed to joining.
	uid := srv.uidHeap.Take()
	srv.rooms[0].Enter(room.SpectatorCID, uid)
	c.SetUID(uid)
	c.SetCID(room.SpectatorCID)
	c.SetRoom(srv.rooms[0])
	c.WriteAO("DONE")
	logger.Debugf("Client joined with UID %v.", uid)

	c.UpdateBackground()
	c.UpdateSides()
	c.UpdateSong()
	c.UpdateAmbiance()
	srv.sendRoomUpdateAllAO(packets.UpdateAll)
}

func (srv *SCServer) handleChangeChars(c *client.Client, contents []string) {
	cid, err := strconv.Atoi(contents[1])
	if err != nil {
		return
	}
	c.ChangeChar(cid)
	// TODO: SpriteChat version
	srv.writeToRoomAO(c.Room(), "CharsCheck", c.Room().TakenList()...)
}

func (srv *SCServer) handleIC(c *client.Client, contents []string) {
    // Welcome to He11. It is time to validate an IC message.
    if (c.CID() == room.SpectatorCID) {
        srv.sendServerMessage(c, "Spectators cannot speak.")
        return
    }
    if (c.MuteState() & client.MutedIC != 0) {
        srv.sendServerMessage(c, "You are IC muted!")
        return
    }
    if (c.Room().LockState() == room.LockSpec && !c.Room().IsInvited(c.UID())) {
        srv.sendServerMessage(c, "This room is in spectatable mode and you are not on the invite list.")
        return
    }

    // The client IC packet can have between 15 and 26 arguments. The server has 30, due to extra information
    // for pairing. The first 16 arguments align exactly between both (if they exist).
    resp := make([]string, 30) 
    copy(resp[:17], contents)
    // TODO: pairing support.
    resp[16] = "-1" // (other_charid) no pairing
    resp[17] = ""   // (other_name)   no pairing showname
    resp[18] = "0"  // (other_emote)  no pairing emote
    resp[20] = "0"  // (other_offset) no pairing offset
    resp[21] = "0"  // (other_flip)   no pairing flip
    // Now, the rest of the arguments are a bit cursed because of the misalignment.
    if (len(contents) >= 19) {
        // TODO: check version for offset.
        resp[19] = contents[17] // (self_offset)
        copy(resp[22:], contents[18:])
    }

    // TODO: actually check for the packet's validity.

    // deskmod
    if resp[0] ==  "chat" {
        // This has been deprecated on newer clients, but we replace it anyhow.
        resp[0] = "1"
    }

    // char name (i.e. the actual file) (resp[2])
    // TODO: add an iniswap check.

    // emote (resp[3])
    // TODO: narrator mode.

    // message
    resp[4] = strings.TrimSpace(resp[4])
    if len(resp[4]) > srv.config.MaxMsgSize {
		srv.sendServerMessage(c, "Your message is too long!")
		return
    }
    // TODO: blankpost check.
    // TODO: doublepost check.

    // pos/side resp[5]
    // TODO: side check.

    // emote mod
    if resp[7] == "4" { // for some reason, this can crash the client.
        resp[7] = "6"
    }

    // char id
    if resp[8] != strconv.Itoa(c.CID()) {
        // incorrect CID
        return
    }

    // shout modifier resp[10]
    // TODO: check custom shout.
    // TODO: check if room allows shouting.

    // evidence
    // TODO: deal with evidence.
    resp[11] = "0" // 0 is the index for no evidence

    // 2.6+ extensions, from here on
    // showname
    resp[15] = strings.TrimSpace(resp[15])
    if (len(resp[15]) > srv.config.MaxNameSize) {
        srv.sendServerMessage(c, "Your showname is too long!")
        return
    }
    c.SetShowname(resp[15])
    // TODO: setting for forbidding shownames in rooms?
    // TODO: allow "blank" showname?

    // non-interrupting preanim ("immediate")
    if resp[22] == "" {
        resp[22] = "0"
    }
    // TODO: akashi and athena do some more funny checking here, maybe i should too
    
    // 2.8+ extensions, from here on
    // sfx looping
    if resp[23] == "" {
        resp[23] = "0"
    }
    // screenshake
    if resp[24] == "" {
        resp[24] = "0"
    }
    // additive
    // TODO: add check for last speaker
    // TODO: study some of the checks akashi does
    if resp[28] == "" { 
        resp[28] = "0"
    } else if resp[28] == "1" {
        var b strings.Builder
        b.WriteString(" ")
        b.WriteString(resp[4])
        resp[4] = b.String()
    }

    srv.writeToRoomAO(c.Room(), "MS", resp...)
}

func (srv *SCServer) handleOOC(c *client.Client, contents []string) {
	if c.MuteState()&client.MutedOOC != 0 {
		srv.sendServerMessage(c, "You are OOC muted!")
		return
	}
	name := contents[0]
	msg := contents[1]

	outMsg := strings.TrimSpace(msg)
	if outMsg == "" {
		srv.sendServerMessage(c, "Cannot send blank OOC message.")
		return
	}
	if len(outMsg) > srv.config.MaxMsgSize {
		srv.sendServerMessage(c, "Your message is too long!")
		return
	}

	outName := strings.TrimSpace(name)
	if outName == "" {
		srv.sendServerMessage(c, "Set a username to send OOC messages!")
		return
	}
	if len(outName) > srv.config.MaxNameSize {
		srv.sendServerMessage(c, "Your username is too long!")
		return
	}
	for cl := range srv.clients.Clients() {
		if cl.Username() == outName && cl != c {
			srv.sendServerMessage(c, fmt.Sprintf("Username '%v' is already in use in the server.", name))
			return
		}

	}
	// TODO: commands!!!

	c.SetUsername(outName)
	srv.sendOOCMessageToRoom(c.Room(), outName, outMsg, false)
	c.Room().LogEvent(room.EventOOC, "%v (CID: %v, UID: %v): %v", outName, c.CID(), c.UID(), outMsg)
}

func (srv *SCServer) handleMusicArea(c *client.Client, contents []string) {
	// Areas/rooms were originally a hack built on top of songs in AO.
	// For this reason, the music packet is used for both areas and music to this day.
	for _, r := range c.Room().VisibleNames() {
		if r == contents[0] {
			srv.handleArea(c, contents)
			return
		}
	}
	for _, s := range c.Room().MusicList() {
		if s == contents[0] {
			srv.handleMusic(c, contents)
			return
		}
	}
}

func (srv *SCServer) handleMusic(c *client.Client, contents []string) {
	if c.MuteState()&client.MutedMusic != 0 {
		srv.sendServerMessage(c, "You are muted from playing music.")
		return
	}
	if (c.Room().LockState() == room.LockSpec) && !c.Room().IsInvited(c.UID()) {
		srv.sendServerMessage(c, "You are only allowed to spectate in this area.")
		return
	}

	song := contents[0]
	if !strings.Contains(song, ".") { // song name is a category, therefore stop
		song = packets.SongStop
	}

	var showname string
	if len(contents) >= 3 {
		showname = strings.TrimSpace(contents[2])
		c.SetShowname(showname)
	}
	if showname == "" {
		showname = c.Room().GetNameByCID(c.CID())
	}

	effects := "0"
	if len(contents) >= 4 {
		effects = contents[3]
	}
	c.Room().SetSong(song)
	srv.writeToRoomAO(c.Room(), "MC", song, contents[1], showname, "1", "0", effects)
	if song == packets.SongStop {
		c.Room().LogEvent(room.EventMusic, "%v (CID: %v, UID: %v) stopped the music.", showname, c.CID(), c.UID())
	} else {
		c.Room().LogEvent(room.EventMusic, "%v (CID: %v, UID: %v) played %v.", showname, c.CID(), c.UID(), song)
	}
	return
}

func (srv *SCServer) handleArea(c *client.Client, contents []string) {
	dst := srv.getRoomByName(contents[0])
	if dst == nil {
		srv.logger.Debugf("Client (UID: %v, IPID: %v) tried joining non-existant room (%v).", c.UID(), c.IPID(), contents[0])
		return
	}
	srv.moveClient(c, dst)
}

func (srv *SCServer) handleCheck(c *client.Client, contents []string) {
	c.WriteAO("CHECK")
}
