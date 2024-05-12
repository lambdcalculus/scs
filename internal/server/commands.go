package server

// TODO: flags
// could be good to make duration optional in a few of the commands

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
	Default targetType = iota // Default type for the command.
	CID
	UID
	IPID
)

// For commands that can optionally be given a 'reason' argument.
const noReason string = "No reason given."

// In case code that should be unreachable is somehow reached.
const unreachableMsg string = "You shouldn't see this message! If you do, please tell the server developer."

// A cmdFunc attempts to execute a command with the passed args. It returns whether
// a message to send the client who issued the command, and two bools: one indicating
// whether the command was successful, and one indicating whether the usage should be sent.
type cmdFunc func(srv *SCServer, c *client.Client, args []string) (reply string, success bool, sendUsage bool)

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
		"manage": {(*SCServer).cmdManage, 0, perms.None,
			"/manage [uids...]\n" +
				"/manage <'cid'|'uid'> <ids...>",
			"Promotes to manager (if allowed). If already promoted, user can promote others. Will use UID to promote others unless otherwise specified."},
		"unmanage": {(*SCServer).cmdUnmanage, 0, perms.None,
			"/unmanage [uids...]\n" +
				"/unmanage <'cid'|'uid'> <ids...>",
			"Demotes user from manager. Only managers can use this command. Will use UID to demote others unless otherwise specified."},
		"bg": {(*SCServer).cmdBackground, 1, perms.Background,
			"/bg <background...>",
			"Changes the room's background."},
		// "ambiance": {(*SCServer).cmdAmbiance, 1, perms.Ambiance,
		// 	"/bg <background...>",
		// 	"Changes the room's ambiance."},
		// /lock
		// /unlock
		// /toggle
		// /invite
		// /uninvite
		// /play
	}
}

func (srv *SCServer) handleCommand(c *client.Client, name string, args []string) {
	cmd, ok := cmdMap[name]
	joinedArgs := strings.Join(args, " ") // for the log messages
	if !ok {
		srv.sendServerMessage(c, fmt.Sprintf("'/%v' is an unknown command. Use /help to see a list of commands.", name))
		c.Room().LogEvent(room.EventFail, "%s tried running unknown command '/%s %s'.",
			c.LongString(), name, joinedArgs)
		return
	}
	if len(args) < cmd.minArgs {
		srv.sendServerMessage(c, fmt.Sprintf("Not enough arguments for /%v.\n Usages of /%v:\n%v", name, name, cmd.usage))
		c.Room().LogEvent(room.EventFail, "%s tried running command '/%s %s' but there are too few arguments.",
			c.LongString(), name, joinedArgs)
		return
	}
	if !c.HasPerms(cmd.reqPerms) {
		// TODO: list required permissions
		srv.sendServerMessage(c, fmt.Sprintf("You do not have the required permisions to use /%v.", name))
		c.Room().LogEvent(room.EventFail, "%s tried running command '/%s %s' but did not have permission.",
			c.LongString(), name, joinedArgs)
		return
	}

	msg, success, sendUsage := cmd.cmdFunc(srv, c, args)
	if success {
		c.Room().LogEvent(room.EventCommand, "%s ran command '/%s %s'.",
			c.LongString(), name, strings.Join(args, " "))
	} else {
		c.Room().LogEvent(room.EventFail, "%s tried to run command '/%s %s' but failed (%s)",
			c.LongString(), name, strings.Join(args, " "), msg)
	}

	// just in case, lmao
	if msg == unreachableMsg {
		srv.logger.Warnf("%s reached supposedly unreachable code with command '/%s %s'.",
			c.LongString(), name, strings.Join(args, " "))
	}

	var reply string
	if msg != "" {
		reply += name + ": "
		reply += msg
	}
	if sendUsage {
		if reply != "" {
			reply += "\n"
		}
		reply += fmt.Sprintf("Usages of /%v:\n%v", name, cmd.usage)
	}
	if reply != "" {
		srv.sendServerMessage(c, reply)
	}
}

func (srv *SCServer) cmdHelp(c *client.Client, args []string) (string, bool, bool) {
	if len(args) == 0 {
		// TODO: make this prettier
		msg := "Available commands:\n"
		for cmd := range cmdMap {
			msg += "/" + cmd + ", "
		}
		return msg[:len(msg)-2], true, false
	}
	cmd, ok := cmdMap[args[0]]
	if !ok {
		return fmt.Sprintf("'%v' is not a valid command.", args[0]), false, false
	}
	return fmt.Sprintf("Usage of /%v:\n%v\nDetails: %v", args[0], cmd.usage, cmd.detailed), true, false
}

func (srv *SCServer) cmdLogin(c *client.Client, args []string) (string, bool, bool) {
	ok, role, err := srv.db.CheckAuth(args[0], args[1])
	if err != nil {
		srv.logger.Warnf("Error in authentication (%v).", err)
		return "Couldn't authenticate: internal error.", false, false
	}
	if !ok {
		return "Incorrect password, or user doesn't exist.", false, false
	}
	for _, r := range srv.roles {
		if r.Name == role {
			c.AddRole(r)
			if r.Perms&perms.HearModCalls != 0 {
				c.AddGuard()
			}
			// TODO: say permissions?
			return fmt.Sprintf("Successfully authenticated as user '%v' and role '%v'.", args[0], role), true, false
		}
	}
	return fmt.Sprintf("Was able to authenticate, but role '%v' doesn't exist.", role), false, false
}

func (srv *SCServer) cmdMute(c *client.Client, args []string) (string, bool, bool) {
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
	var t targetType
	t = parseTarget(args[0])
	if t != Default {
		args = args[1:]
	} else {
		t = UID
	}

	// now the next 3 arguments are ID, duration and, optionally, reason
	dur, err := duration.ParseDuration(args[1])
	if err != nil {
		return fmt.Sprintf("''%s' is not a valid duration: %s.", args[1], err), false, true
	}

	var reason string
	if len(args) < 3 {
		reason = noReason
	} else {
		reason = strings.Join(args[2:], " ")
	}

	targets, err := srv.getTargets(c, t, args[0:1])
	if err != nil {
		return err.Error(), false, false
	}

	var msg strings.Builder
	var muted strings.Builder
	muted.WriteString("Successfully muted ")
	first := true
	for _, cl := range targets {
		// cannot mute a client that has the same or more privileges than you
		if c.Perms().Subset(cl.Perms()) {
			msg.WriteString(fmt.Sprintf("Can't mute %s, they have the same privileges as you, or more.\n", cl.ShortString()))
			continue
		}

		cl.AddMute(mute, dur)
		srv.sendServerMessage(cl, "You have been muted%s for %s for: %s", from, args[1], reason)

		if err := srv.db.AddMute(cl.IPID(), cl.Ident(), reason, c.Username(), dur); err != nil {
			srv.logger.Warnf("Couldn't add mute to the database (%s).", err)
		}

		if first {
			muted.WriteString(fmt.Sprintf("%v", cl.ShortString()))
			first = false
		} else {
			muted.WriteString(fmt.Sprintf(", %v", cl.ShortString()))
		}
	}

	if first { // if this is still true, couldn't mute anyone
		msg.WriteString("Couldn't mute any client.")
		return msg.String(), false, false
	}
	muted.WriteString(fmt.Sprintf("%s for %s.", from, args[1]))
	msg.WriteString(muted.String())
	return msg.String(), true, false
}

func (srv *SCServer) cmdUnmute(c *client.Client, args []string) (string, bool, bool) {
	// first, check if it's specifying a unmute. if it is, consume an argument
	var unmute client.MuteState
	var from string
	switch strings.ToLower(args[0]) {
	case "ic":
		args = args[1:]
		unmute = client.MutedIC
		from = " from IC chat"
	case "ooc":
		args = args[1:]
		unmute = client.MutedOOC
		from = " from OOC chat"
	case "jud":
		args = args[1:]
		unmute = client.MutedJudge
		from = " from using judge commands"
	case "music":
		args = args[1:]
		unmute = client.MutedMusic
		from = " from playing music"
	case "all":
		args = args[1:]
		fallthrough
	default:
		unmute = client.MutedAll
		from = ""
	}

	// now, check for a target type. if specified, consume an argument
	var t targetType
	t = parseTarget(args[0])
	if t != Default {
		args = args[1:]
	} else {
		t = UID
	}

	// now the next argument is ID
	targets, err := srv.getTargets(c, t, args[0:1])
	if err != nil {
		return err.Error(), false, false
	}

	var unmuted strings.Builder
	unmuted.WriteString("Successfully unmuted ")
	first := true
	for _, cl := range targets {
		cl.RemoveMute(unmute)
		srv.sendServerMessage(cl, "You have been unmuted%s.", from)

		if first {
			unmuted.WriteString(fmt.Sprintf("%v", cl.ShortString()))
			first = false
		} else {
			unmuted.WriteString(fmt.Sprintf(", %v", cl.ShortString()))
		}
	}
	unmuted.WriteString(fmt.Sprintf("%s.", from))
	return unmuted.String(), true, false
}

func (srv *SCServer) cmdKick(c *client.Client, args []string) (string, bool, bool) {
	// check if target type is specified. if it is, consume an argument
	var t targetType
	t = parseTarget(args[0])
	if t != Default {
		args = args[1:]
	} else {
		t = UID
	}

	// now the next 2 arguments are ID, and optionally reason
	var reason string
	if len(args) < 2 {
		reason = noReason
	} else {
		reason = strings.Join(args[1:], " ")
	}

	targets, err := srv.getTargets(c, t, args[0:1])
	if err != nil {
		return err.Error(), false, false
	}

	var msg strings.Builder
	var kicked strings.Builder
	kicked.WriteString("Successfully kicked ")
	first := true
	for _, cl := range targets {
		// cannot kick a client that has the same or more privileges than you
		if c.Perms().Subset(cl.Perms()) {
			msg.WriteString(fmt.Sprintf("Can't kick %s, they have the same privileges as you, or more.\n", cl.ShortString()))
			continue
		}

		srv.kickClient(cl, reason)
		if err := srv.db.AddKick(cl.IPID(), cl.Ident(), reason, c.Username()); err != nil {
			srv.logger.Warnf("Couldn't add kick to the database: %s", err)
		}

		if first {
			kicked.WriteString(fmt.Sprintf("%v", cl.ShortString()))
			first = false
		} else {
			kicked.WriteString(fmt.Sprintf(", %v", cl.ShortString()))
		}
	}

	if first { // if this is true, couldn't kick anyone
		msg.WriteString("Couldn't kick any client.")
		return msg.String(), false, false
	}
	kicked.WriteString(fmt.Sprintf(" for reason: %s.", reason))
	msg.WriteString(kicked.String())

	return msg.String(), true, false
}

func (srv *SCServer) cmdBan(c *client.Client, args []string) (string, bool, bool) {
	// TODO: add flag for explicitly banned offline targets
	// check if target type is specified. if it is, consume an argument.
	var t targetType
	t = parseTarget(args[0])
	if t != Default {
		args = args[1:]
	} else {
		t = UID
	}

	// now the next 3 arguments are ID, duration, and reason

	// TODO: default duration? needs a duration flag, probs
	var dur time.Duration
	var err error
	if args[1] == "perma" {
		dur = time.Duration(math.MaxInt64)
	} else if dur, err = duration.ParseDuration(args[1]); err != nil {
		return fmt.Sprintf("'%s' is not a valid duration: %s.", args[1], err), false, false
	}

	reason := strings.Join(args[2:], " ")

	var ipid string
	targets, err := srv.getTargets(c, t, args[0:1])
	if err != nil {
		if t != IPID {
			return err.Error(), false, false
		}
		// No client online with the passed IPID - we'll add a ban record.
		// TODO: add a flag for this, to avoid people getting banned by typos
		var msg strings.Builder
		ipid = args[0]
		msg.WriteString(fmt.Sprintf("No clients currently online with IPID %s. Adding a ban record for this IPID.\n", args))
		if err := srv.db.AddBan(ipid, "", reason, c.Username(), dur); err != nil {
			srv.logger.Warnf("Couldn't add ban (%s).", err)
			return "Database error. Warn the host!", false, false
		}
		msg.WriteString(fmt.Sprintf("Successfully banned IPID %s.", ipid))
		return msg.String(), true, false
	}
	ipid = targets[0].IPID() // not empty if we got here

	banMsg := fmt.Sprintf("You have been banned. Reason: %s (until %s)", reason, time.Now().Add(dur).UTC().Format(time.UnixDate))
	var hdids []string // we don't want to add repeat HDIDs to the ban records
	var msg strings.Builder
	var banned strings.Builder
	banned.WriteString("Successfully banned ")
	first := true
	for _, cl := range targets {
		// can't ban clients with the same or more permissions
		if c.Perms().Subset(cl.Perms()) {
			msg.WriteString(fmt.Sprintf("Can't ban %s, they have the same privileges as you, or more.\n", cl.ShortString()))
			continue
		}

		// check for new HDID
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

		if first {
			banned.WriteString(fmt.Sprintf("%v", c.ShortString()))
			first = false
		} else {
			banned.WriteString(fmt.Sprintf(", %v", c.ShortString()))
		}
	}

	if first { // if this is still true, couldn't ban anyone
		msg.WriteString("Couldn't ban any client.")
		return msg.String(), false, false
	}
	banned.WriteString(fmt.Sprintf(" for %s for reason: %s.", duration.String(dur), reason))

	for _, hdid := range hdids {
		if err := srv.db.AddBan(targets[0].IPID(), hdid, reason, c.Username(), dur); err != nil {
			srv.logger.Warnf("Couldn't add ban (%s).", err)
			msg.WriteString("Database error. Warn the host!")
			return msg.String(), false, false
		}
	}

	msg.WriteString(banned.String())
	return msg.String(), true, false
}

func (srv *SCServer) cmdGet(c *client.Client, args []string) (string, bool, bool) {
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
		return msg, true, false

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
		return msg, true, false

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
		return msg, true, false
	default:
		return "Invalid argument.", false, true
	}
}

// TODO: remove manager privileges when moving rooms
func (srv *SCServer) cmdManage(c *client.Client, args []string) (string, bool, bool) {
	if len(args) == 0 {
		// promoting self
		if len(c.Room().Managers()) != 0 && !c.HasPerms(perms.BypassLocks) {
			return "This room already has a manager. Ask them to promote you.", false, false
		}
		if !c.Room().AllowManagers() && !c.HasPerms(perms.BypassLocks) {
			return "Promoting to manager is not allowed in this room.", false, false
		}
		if c.Room().IsManager(c.UID()) {
			return "You are already a manager in this room!", false, false
		}

		c.Room().AddManager(c.UID())
		c.AddRole(srv.mgrRole)
		srv.sendServerMessageToRoom(c.Room(), "%s is now managing this room.", c.ShortString())
		return fmt.Sprintf("Promoted to '%s'!", srv.mgrRole.Name), true, false
	}

	// if we're here, then the user is trying to promote others
	if !c.Room().IsManager(c.UID()) {
		return "You must be a manager yourself to promote others.", false, false
	}

	// check if first argument is target type. if it is, consume it
	t := parseTarget(args[0])
	if t != Default {
		args = args[1:]
	} else {
		t = UID
	}
	if t == IPID {
		return "Can't promote by IPID.", false, true
	}

	// now the only arguments left are the ids
	targets, err := srv.getTargets(c, t, args[:])
	if err != nil {
		return err.Error(), false, false
	}

	var msg strings.Builder
	var promoted strings.Builder
	promoted.WriteString("Successfully promoted ")
	first := true
	for _, cl := range targets {
		if cl.Room() != c.Room() {
			msg.WriteString(fmt.Sprintf("%s is not in this room. Skipping.\n", cl.ShortString()))
			continue
		}
		if cl.Room().IsManager(cl.UID()) {
			msg.WriteString(fmt.Sprintf("%s is already a manager in this room. Skipping.\n", cl.ShortString()))
			continue
		}
		cl.AddRole(srv.mgrRole)
		cl.Room().AddManager(cl.UID())
		srv.sendServerMessageToRoom(cl.Room(), "%s is now managing this room.", cl.ShortString())

		if first {
			promoted.WriteString(fmt.Sprintf("%v", c.ShortString()))
		} else {
			promoted.WriteString(fmt.Sprintf(", %v", c.ShortString()))
		}
	}
	promoted.WriteString(".")

	if first { // if this is still true, couldn't promote anyone
		msg.WriteString("Couldn't promote any client.")
		return msg.String(), false, false
	}

	msg.WriteString(promoted.String())
	return msg.String(), true, false
}

func (srv *SCServer) cmdUnmanage(c *client.Client, args []string) (string, bool, bool) {
	if len(args) == 0 {
		// demoting self
		if !c.Room().IsManager(c.UID()) {
			return "You are not a manager!", false, false
		}

		c.Room().RemoveManager(c.UID())
		c.RemoveRole(srv.mgrRole)
		srv.sendServerMessageToRoom(c.Room(), "%s is no longer managing this room.", c.ShortString())
		return fmt.Sprintf("No longer '%s'!", srv.mgrRole.Name), true, false
	}

	// if we're here, then the user is trying to demote others
	if !c.Room().IsManager(c.UID()) {
		return "You must be a manager yourself to demote others.", false, false
	}

	// check if first argument is t type. if it is, consume it
	t := parseTarget(args[0])
	if t != Default {
		args = args[1:]
	} else {
		t = UID
	}
	if t == IPID {
		return "Cannot demote by IPID", false, true
	}

	// now the only arguments left are the ids
	targets, err := srv.getTargets(c, t, args[:])
	if err != nil {
		return err.Error(), false, false
	}

	var msg strings.Builder
	var demoted strings.Builder
	demoted.WriteString("Successfully demoted ")
	first := true
	for _, cl := range targets {
		if !c.Room().IsManager(cl.UID()) {
			msg.WriteString(fmt.Sprintf("%s is not a manager in this room.\n", cl.ShortString()))
			continue
		}

		cl.RemoveRole(srv.mgrRole)
		c.Room().RemoveManager(cl.UID())
		srv.sendServerMessageToRoom(cl.Room(), "%s is no longer managing this room.", cl.ShortString())

		if first {
			demoted.WriteString(fmt.Sprintf("%v", cl.ShortString()))
		} else {
			demoted.WriteString(fmt.Sprintf(", %v", cl.ShortString()))
		}
	}
	demoted.WriteString(".")

	if first { // if this is still true, couldn't promote anyone
		msg.WriteString("Couldn't demote any client.")
		return msg.String(), false, false
	}

	msg.WriteString(demoted.String())
	return msg.String(), true, false
}

func (srv *SCServer) cmdBackground(c *client.Client, args []string) (string, bool, bool) {
	return "lol", true, false
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

// Gets the clients targeted by a command.
func (srv *SCServer) getTargets(c *client.Client, t targetType, ids []string) ([]*client.Client, error) {
	var clients []*client.Client
	switch t {
	case UID:
		for _, id := range ids {
			uid, err := strconv.Atoi(id)
			if err != nil {
				return nil, fmt.Errorf("'%v' is not a valid UID.", id)
			}
			cl := srv.getByUID(uid)
			if c == nil {
				return nil, fmt.Errorf("No client with UID %v.", uid)
			}
			clients = append(clients, cl)
		}
	case CID:
		cls := srv.getClientsInRoom(c.Room())
		for _, id := range ids {
			cid, err := strconv.Atoi(id)
			if err != nil {
				return nil, fmt.Errorf("'%v' is not a valid UID.", cid)
			}
			var found bool
			for _, cl := range cls {
				if cl.CID() == cid {
					found = true
					clients = append(clients, cl)
				}
			}
			if !found {
				return nil, fmt.Errorf("No client with CID %v in this room.", cid)
			}
		}
	case IPID:
		for _, ipid := range ids {
			cl := srv.getByIPID(ipid)
			if cl == nil {
				return nil, fmt.Errorf("No client with IPID '%s'.", ipid)
			}
			clients = append(clients, cl...)
		}
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("No targets found.")
	}
	return clients, nil
}
