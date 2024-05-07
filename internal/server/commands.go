package server

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lambdcalculus/scs/internal/client"
	"github.com/lambdcalculus/scs/internal/perms"
	"github.com/lambdcalculus/scs/internal/room"
)

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
			"/help [command: optional]",
			"Shows detailed usage of a command, or the list of commands if no command is passed."},
		"login": {(*SCServer).cmdLogin, 2, perms.None,
			"/login [username] [password]",
			"Attempts to authenticate with the passed username and password."},
		"kick": {(*SCServer).cmdKick, 2, perms.Kick,
			"/kick <cid|uid|ipid> [id] [reason: optional]",
			"Kicks an user by CID, UID or IPID with an optional reason. Note that kicking by IPID kicks all instances of that IPID - to kick a specific client, kick by UID or CID.\n" +
				"Example usage: /kick uid 1 dumb and stupid\""},
		"get": {(*SCServer).cmdGet, 1, perms.None,
			"/get <room|rooms|allrooms>",
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
		srv.sendServerMessage(c, fmt.Sprintf("Not enough arguments for /%v.\n Usage of /%v: %v", name, name, cmd.usage))
		c.Room().LogEvent(room.EventFail, "%s tried running command '/%s' with too few arguments %#v.",
			c.LongString(), name, args)
		return
	}
	if !c.HasPerms(cmd.reqPerms) {
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
		reply += fmt.Sprintf("Usage of /%v: %v", name, cmd.usage)
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
	return fmt.Sprintf("Usage of /%v: %v\n%v", args[0], cmd.usage, cmd.detailed), false
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
func (srv *SCServer) cmdKick(c *client.Client, args []string) (string, bool) {
	var reason string
	if len(args) < 3 {
		reason = "No reason given."
	} else {
		reason = strings.Join(args[2:], " ")
	}

	switch args[0] {
	case "ipid":
		ipid := args[1]
		toKick := srv.getByIPID(ipid)
		if toKick == nil {
			return fmt.Sprintf("No client with IPID '%v'.", ipid), false
		}
		for _, cl := range toKick {
			srv.kickClient(cl, reason)
		}
		return fmt.Sprintf("Successfully kicked client with IPID %v.", ipid), false

	case "cid":
		cid, err := strconv.Atoi(args[1])
		// TODO: check for Spectator?
		if err != nil {
			return fmt.Sprintf("'%v' is not a valid CID.", args[1]), false
		}
		for _, cl := range srv.getClientsInRoom(c.Room()) {
			if cl.CID() == cid {
				srv.kickClient(cl, reason)
				return fmt.Sprintf("Successfully kicked client with CID %v.", cid), false
			}
		}
		return fmt.Sprintf("No client with CID %v in this room.", cid), false

	case "uid":
		uid, err := strconv.Atoi(args[1])
		if err != nil {
			return fmt.Sprintf("'%v' is not a valid UID.", args[1]), false
		}
		toKick := srv.getByUID(uid)
		if toKick == nil {
			return fmt.Sprintf("No client with UID '%v'.", uid), false
		}
		srv.kickClient(toKick, reason)
		return fmt.Sprintf("Successfully kicked client with UID %v.", uid), false

	default:
		return "First argument must be 'ipid', 'cid', or 'uid'.", true
	}
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
