package server

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/lambdcalculus/scs/internal/client"
	"github.com/lambdcalculus/scs/internal/perms"
	"github.com/lambdcalculus/scs/internal/room"
	"github.com/lambdcalculus/scs/pkg/duration"
)

// Enum type for commands that target a user (i.e. can be targeted by CID, UID or IPID).
// Also includes Default, to use a command's default target type.
type targetType int

const (
	// Default type for the command.
	Default targetType = iota
	CID
	UID
	IPID
)

// For commands that can optionally be given a 'reason' argument.
const noReason string = "No reason given."

// In case code that should be unreachable is somehow reached.
const unreachableMsg string = "You shouldn't see this message! If you do, please tell the server developer."

// A cmdFunc attempts to execute a command with the passed args. It returns whether
// the command's usage should be sent, along with a message to send to the user of
// the command.
type cmdFunc func(srv *SCServer, c *client.Client, args []string) (string, bool)

type cmdHandler struct {
	cmdFunc  cmdFunc
	minArgs  int
	reqPerms perms.Mask
	usage    string
	detailed string
}

var cmdMap map[string]cmdHandler

func init() {
	cmdMap = map[string]cmdHandler{
		"help": {(*SCServer).cmdHelp, 0, perms.None,
			"/help [command]",
			"Shows detailed usage of a command, or the list of commands if no command is passed."},

		// moderation
		"login": {(*SCServer).cmdLogin, 2, perms.None,
			"/login <username> <password>",
			"Attempts to authenticate with the passed username and password."},
		"mute": {(*SCServer).cmdMute, 2, perms.Mute,
			"/mute <uid> <duration> [reason...]\n" +
				"/mute <'ic'|'ooc'|'jud'|'music'|'all'> <uid> <duration> [reason...]\n" +
				"/mute <'cid'|'uid'|'ipid'> <id> <duration> [reason...]\n" +
				"/mute <'ic'|'ooc'|'jud'|'music'|'all'> <'cid'|'uid'|'ipid'> <id> <duration> [reason...]",
			"Mutes a user for the specified duration with an optional reason. Mutes user in all of IC/OOC/judge/music unless otherwise specified. Mutes by UID unless otherwise specified. Duration should be in a format like '2h30m' or '3d12h'. Note: if muting by IPID, all clients with that IPID will be muted."},
		"kick": {(*SCServer).cmdKick, 1, perms.Kick,
			"/kick <uid> [reason...]\n" +
				"/kick <'cid'|'uid'|'ipid'> <id> [reason...]",
			"Kicks a user with an optional reason. Kicks by UID unless otherwise specified. Note: if kicking by IPID, all clients with that IPID will be kicked."},
		"ban": {(*SCServer).cmdBan, 3, perms.Ban,
			"/ban <ipid> <duration> <reason...>\n" +
				"/ban <'cid'|'uid'|'ipid'> <id> <duration> <reason...>",
			"Bans a user for the specified duration. Reason is required. Bans by IPID unless otherwise specified. Duration should be in a format like '2h30m' or '3d12h'. Duration can be 'perma' for permanent ban."},

		// rooms
		"get": {(*SCServer).cmdGet, 1, perms.None,
			"/get <'room'|'rooms'|'allrooms'>",
			"Gets a list of users in a room or set of rooms. Use:\n" +
				"\"/get room\" to get a list of users in the same room as you;\n" +
				"\"/get rooms\" to get a list of users in the rooms that you can see;\n" +
				"\"/get allrooms\" to get a list of all users in the server."},
	}
}

func (srv *SCServer) handleCommand(c *client.Client, name string, args []string) {
	cmd, ok := cmdMap[name]
	if !ok {
		srv.sendServerMessage(c, fmt.Sprintf("'/%v' is an unknown command. Use /help to see a list of commands.", name))
		c.Room().LogEvent(room.EventFail, "%s tried running unknown command '/%s' with arguments %#v",
			c.LongString(), name, args)
		return
	}
	if len(args) < cmd.minArgs {
		srv.sendServerMessage(c, fmt.Sprintf("Not enough arguments for /%v.\n Usages of /%v:\n%v", name, name, cmd.usage))
		c.Room().LogEvent(room.EventFail, "%s tried running command '/%s' with too few arguments %#v.",
			c.LongString(), name, args)
		return
	}
	if !c.HasPerms(cmd.reqPerms) {
		// TODO: list required permissions
		srv.sendServerMessage(c, fmt.Sprintf("You do not have the required permisions to use /%v.", name))
		c.Room().LogEvent(room.EventFail, "%s tried running command '/%s' with arguments %#v but did not have permission.",
			c.LongString(), name, args)
		return
	}
	c.Room().LogEvent(room.EventCommand, "%s ran command '/%s' with arguments %#v.", c.LongString(), name, args)
	msg, usage := cmd.cmdFunc(srv, c, args)
	var reply string
	if msg != "" {
		reply += msg
	}
	if usage {
		if reply != "" {
			reply += "\n"
		}
		reply += fmt.Sprintf("Usages of /%v:\n%v", name, cmd.usage)
	}
	if reply != "" {
		srv.sendServerMessage(c, reply)
	}
}

func (srv *SCServer) cmdHelp(c *client.Client, args []string) (string, bool) {
	if len(args) == 0 {
		// TODO: make this prettier
		msg := "Available commands:\n"
		for cmd := range cmdMap {
			msg += "/" + cmd + ", "
		}
		return msg[:len(msg)-2], false
	}
	cmd, ok := cmdMap[args[0]]
	if !ok {
		return fmt.Sprintf("'%v' is not a valid command.", args[0]), false
	}
	return fmt.Sprintf("Usage of /%v:\n%v\nDetails: %v", args[0], cmd.usage, cmd.detailed), false
}

func (srv *SCServer) cmdLogin(c *client.Client, args []string) (string, bool) {
	ok, role, err := srv.db.CheckAuth(args[0], args[1])
	if err != nil {
		srv.logger.Warnf("Error in authentication (%v).", err)
		return "Couldn't authenticate: internal error.", false
	}
	if !ok {
		return "Incorrect password, or user doesn't exist.", false
	}
	for _, r := range srv.roles {
		if r.Name == role {
			c.SetPerms(r.Perms)
			if r.Perms&perms.HearModCalls != 0 {
				c.AddGuard()
			}
			// TODO: say permissions?
			return fmt.Sprintf("Successfully authenticated as user '%v' and role '%v'.", args[0], role), false
		}
	}
	return fmt.Sprintf("Was able to authenticate, but role '%v' doesn't exist.", role), false
}

func (srv *SCServer) cmdMute(c *client.Client, args []string) (string, bool) {
	// first, check if it's specifying a mute. if it is, consume an argument
	var mute client.MuteState
	var from string
	switch strings.ToLower(args[0]) {
	case "ic":
		args = args[1:]
		mute = client.MutedIC
		from = " from IC chat"
	case "ooc":
		args = args[1:]
		mute = client.MutedOOC
		from = " from OOC chat"
	case "jud":
		args = args[1:]
		mute = client.MutedJudge
		from = " from using judge commands"
	case "music":
		args = args[1:]
		mute = client.MutedMusic
		from = " from playing music"
	case "all":
		args = args[1:]
		fallthrough
	default:
		mute = client.MutedAll
		from = ""
	}
	// now, check for a target type. if specified, consume an argument
	var target targetType
	target = parseTarget(args[0])
	if target != Default {
		args = args[1:]
	}

	// now the next 3 arguments are ID, duration and, optionally, reason
	dur, err := duration.ParseDuration(args[1])
	if err != nil {
		return fmt.Sprintf("''%s' is not a valid duration: %s.", args[1], err), true
	}

	var reason string
	if len(args) < 3 {
		reason = noReason
	} else {
		reason = strings.Join(args[2:], " ")
	}

	switch target {
	case Default:
		fallthrough
	case UID:
		uid, err := strconv.Atoi(args[0])
		if err != nil {
			// We send the usage here, in case the first argument was gibberish.
			return fmt.Sprintf("'%v' is not a valid UID.", args[1]), true
		}
		toMute := srv.getByUID(uid)
		if toMute == nil {
			return fmt.Sprintf("No client with UID '%v'.", uid), false
		}
		toMute.AddMute(mute, dur)
		srv.sendServerMessage(toMute, "You have been muted%s for %s for the reason: %s", from, args[1], reason)
		if err := srv.db.AddMute(toMute.IPID(), toMute.Ident(), reason, c.Username(), dur); err != nil {
			srv.logger.Warnf("Couldn't add mute to the database (%s).", err)
		}
		return fmt.Sprintf("Successfully muted client with UID %v for %s.", uid, args[1]), false

	case CID:
		cid, err := strconv.Atoi(args[0])
		// TODO: check for Spectator?
		if err != nil {
			return fmt.Sprintf("'%v' is not a valid CID.", args[1]), false
		}
		for _, cl := range srv.getClientsInRoom(c.Room()) {
			if cl.CID() == cid {
				cl.AddMute(mute, dur)
				srv.sendServerMessage(cl, "You have been muted%s for %s for the reason: %s", from, args[1], reason)
				if err := srv.db.AddMute(cl.IPID(), cl.Ident(), reason, c.Username(), dur); err != nil {
					srv.logger.Warnf("Couldn't add mute to the database (%s).", err)
				}
				return fmt.Sprintf("Successfully muted client with CID %v for %s.", cid, args[1]), false
			}
		}
		return fmt.Sprintf("No client with CID %v in this room.", cid), false

	case IPID:
		ipid := args[0]
		toMute := srv.getByIPID(ipid)
		if toMute == nil {
			return fmt.Sprintf("No client with IPID '%v'.", ipid), false
		}
		for _, cl := range toMute {
			cl.AddMute(mute, dur)
			srv.sendServerMessage(cl, "You have been muted from%s for %s for the reason: %s", from, args[1], reason)
		}
		if err := srv.db.AddMute(ipid, toMute[0].Ident(), reason, c.Username(), dur); err != nil {
			srv.logger.Warnf("Couldn't add mute to the database (%s).", err)
		}
		return fmt.Sprintf("Successfully muted clients with IPID %v for %s.", ipid, args[1]), false
	}

	// unreachable
	return unreachableMsg, false
}

func (srv *SCServer) cmdKick(c *client.Client, args []string) (string, bool) {
	// check if target type is specified. if it is, consume an argument
	var target targetType
	target = parseTarget(args[0])
	if target != Default {
		args = args[1:]
	}

	// now the next 2 arguments are ID, and optionally reason
	var reason string
	if len(args) < 2 {
		reason = noReason
	} else {
		reason = strings.Join(args[1:], " ")
	}

	switch target {
	case Default:
		fallthrough
	case UID:
		uid, err := strconv.Atoi(args[0])
		if err != nil {
			// We send the usage here, in case the first argument was gibberish.
			return fmt.Sprintf("'%v' is not a valid UID.", args[0]), true
		}
		toKick := srv.getByUID(uid)
		if toKick == nil {
			return fmt.Sprintf("No client with UID '%v'.", uid), false
		}
		if err := srv.db.AddKick(toKick.IPID(), toKick.Ident(), reason, c.Username()); err != nil {
			srv.logger.Warnf("Couldn't add kick to the database: %s", err)
		}
		srv.kickClient(toKick, reason)
		return fmt.Sprintf("Successfully kicked client with UID %v.", uid), false

	case CID:
		cid, err := strconv.Atoi(args[0])
		// TODO: check for Spectator?
		if err != nil {
			return fmt.Sprintf("'%v' is not a valid CID.", args[0]), false
		}
		for _, cl := range srv.getClientsInRoom(c.Room()) {
			if cl.CID() == cid {
				srv.kickClient(cl, reason)
				if err := srv.db.AddKick(cl.IPID(), cl.Ident(), reason, c.Username()); err != nil {
					srv.logger.Warnf("Couldn't add kick to the database: %s", err)
				}
				return fmt.Sprintf("Successfully kicked client with CID %v.", cid), false
			}
		}
		return fmt.Sprintf("No client with CID %v in this room.", cid), false

	case IPID:
		ipid := args[0]
		toKick := srv.getByIPID(ipid)
		if toKick == nil {
			return fmt.Sprintf("No client with IPID '%v'.", ipid), false
		}
		for _, cl := range toKick {
			srv.kickClient(cl, reason)
		}
		if err := srv.db.AddKick(ipid, toKick[0].Ident(), reason, c.Username()); err != nil {
			srv.logger.Warnf("Couldn't add kick to the database: %s", err)
		}
		return fmt.Sprintf("Successfully kicked clients with IPID %v.", ipid), false
	}

	// unreachable
	return unreachableMsg, false
}

func (srv *SCServer) cmdBan(c *client.Client, args []string) (string, bool) {
	// check if target type is specified. if it is, consume an argument.
	var target targetType
	target = parseTarget(args[0])
	if target != Default {
		args = args[1:]
	}

	// now the next 3 arguments are ID, duration, and reason
	var ipid string
	switch target {
	case Default:
		fallthrough
	case IPID:
		ipid = args[0]

	case CID:
		cid, err := strconv.Atoi(args[0])
		// TODO: check for Spectator?
		if err != nil {
			return fmt.Sprintf("'%v' is not a valid CID.", args[0]), false
		}
		found := false
		for _, cl := range srv.getClientsInRoom(c.Room()) {
			if cl.CID() == cid {
				found = true
				ipid = cl.IPID()
			}
		}
		if !found {
			return fmt.Sprintf("No client with CID %v in this room.", cid), false
		}

	case UID:
		uid, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Sprintf("'%s' is not a valid UID.", args[0]), false
		}
		cl := srv.getByUID(uid)
		if cl == nil {
			return fmt.Sprintf("No client with UID %v.", uid), false
		}
		ipid = cl.IPID()
	}

	// TODO: default duration?
	var dur time.Duration
	var err error
	if args[1] == "perma" {
		dur = time.Duration(math.MaxInt64)
	} else if dur, err = duration.ParseDuration(args[1]); err != nil {
		return fmt.Sprintf("'%s' is not a valid duration: %s.", args[1], err), false
	}

	reason := strings.Join(args[2:], " ")

	var outMsg string
	toBan := srv.getByIPID(ipid)
	if toBan == nil {
		outMsg += fmt.Sprintf("note: No clients currently online with IPID %s, adding ban record with only IPID.\n", ipid)
		if err := srv.db.AddBan(ipid, "", reason, c.Username(), dur); err != nil {
			srv.logger.Warnf("Couldn't add ban (%s).", err)
			return "Database error. Warn the host!", false
		}
		outMsg += fmt.Sprintf("Successfully banned IPID %s.", ipid)
		return outMsg, false
	}

	banMsg := fmt.Sprintf("You have been banned. Reason: %s (until %s)", reason, time.Now().Add(dur).UTC().Format(time.UnixDate))
	var hdids []string
	for _, cl := range toBan {
		newHDID := true
		for _, hdid := range hdids {
			if cl.Ident() == hdid {
				newHDID = false
			}
		}
		if newHDID {
			hdids = append(hdids, cl.Ident())
		}
		srv.kickClient(cl, banMsg)
	}

	for _, hdid := range hdids {
		if err := srv.db.AddBan(ipid, hdid, reason, c.Username(), dur); err != nil {
			srv.logger.Warnf("Couldn't add ban (%s).", err)
			return "Database error. Warn the host!", false
		}
	}

	return fmt.Sprintf("Successfully banned IPID %s and %v HDIDs.", ipid, len(hdids)), false
}

func (srv *SCServer) cmdGet(c *client.Client, args []string) (string, bool) {
	switch args[0] {
	// TODO: permissions and stuff
	case "room":
		msg := fmt.Sprintf("\n>>> [%v] %v: <<<", c.Room().ID(), c.Room().Name())
		for _, cl := range srv.getClientsInRoom(c.Room()) {
			msg += "\n"
			if c.HasPerms(perms.SeeIPIDs) {
				msg += cl.LongString()
			} else {
				msg += cl.String()
			}
		}
		return msg, false

	case "rooms":
		var msg string
		for _, r := range c.Room().Visible() {
			var submsg string
			submsg += fmt.Sprintf("\n>>> [%v] %v: <<<", r.ID(), r.Name())
			for _, cl := range srv.getClientsInRoom(r) {
				submsg += "\n"
				if c.HasPerms(perms.SeeIPIDs) {
					submsg += cl.LongString()
				} else {
					submsg += cl.String()
				}
			}
			msg += submsg
		}
		return msg, false

	case "allrooms":
		var msg string
		for _, r := range srv.rooms {
			var submsg string
			submsg += fmt.Sprintf("\n>>> [%v] %v: <<<", r.ID(), r.Name())
			for _, cl := range srv.getClientsInRoom(r) {
				submsg += "\n"
				if c.HasPerms(perms.SeeIPIDs) {
					submsg += cl.LongString()
				} else {
					submsg += cl.String()
				}
			}
			msg += submsg
		}
		return msg, false
	default:
		return "", true
	}
}

// Parses a target. Returns Default if no matches are found.
func parseTarget(s string) targetType {
	switch strings.ToLower(s) {
	case "cid":
		return CID
	case "uid":
		return UID
	case "ipid":
		return IPID
	default:
		return Default
	}
}
