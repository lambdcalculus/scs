// Package `room` implements areas/locations.
package room

// TODO: improve logging

import (
	"fmt"
	"strings"
	"sync"

	"github.com/lambdcalculus/scs/internal/config"
	"github.com/lambdcalculus/scs/pkg/logger"
	"github.com/lambdcalculus/scs/pkg/packets"
)

// Clients may join rooms without taking up characters if they join as spectator.
// The spectator CID is -1.
const SpectatorCID = -1

// The "status" of a Room, as in AO.
type Status int

const (
	StatusIdle Status = iota
	StatusLooking
	StatusCasing
	StatusRecess
	StatusRoleplay
	StatusGaming
)

var statusToString = map[Status]string{
	StatusIdle:     "IDLE",
	StatusLooking:  "LOOKING-FOR-PLAYERS",
	StatusCasing:   "CASING",
	StatusRecess:   "RECESS",
	StatusRoleplay: "RP",
	StatusGaming:   "GAMING",
}

// The "lock state" of a Room, as in AO.
type LockState int

const (
	// All users can enter and speak.
	LockFree LockState = iota

	// All users can enter, speech is invite-only.
	LockSpec

	// Only invited users can enter.
	LockLocked
)

var lockToString = map[LockState]string{
	LockFree:   "FREE",
	LockSpec:   "SPECTATABLE",
	LockLocked: "LOCKED",
}

// Used internally to represent an invalid user.
const invalidUID = 0

// A Room represents a single location where clients can be, in the sense that IC/OOC messages
// are sent according to the Room in which a client is in.
type Room struct {
	id       int
	name     string
	desc     string
	adjacent []*Room
	chars    []*char
	music    []MusicCategory
	sides    []string

	blankposting bool
	iniswapping  bool
	shouting     bool
	immediate    bool

	// TODO: evidence? i kinda hate evidence
	// TODO: CMs (and permissions in general)
	// TODO: judge stuff

	song     string
	bg       string
	lockBg   bool
	ambiance string
	lockAmb  bool
	status   Status
	lock     LockState

	// could be another set...
	users       []*user
	lastSpeaker int // CID

	// A list of invited UIDs. Used to decide who can speak when the room spectatable,
	// or who can enter when it is locked.
	invited map[int]struct{} // Another set!

	logger *logger.Logger
	mu     sync.Mutex
}

type char struct {
	name  string
	taken bool
}

type MusicCategory config.SongCategory

type user struct {
	charID int
	userID int
}

// Represents types of occurrences in the Room. They are used for logging.
type Event int

const (
	EventConfig Event = iota
	EventEnter
	EventExit
	EventCharacter
	EventMusic
	EventOOC
	EventIC
	EventJudge
	EventDebug
	EventFail
)

var eventToString = map[Event]string{
	EventConfig:    "CONF ",
	EventEnter:     "ENTER",
	EventExit:      "EXIT ",
	EventCharacter: "CHAR ",
	EventMusic:     "MUSIC",
	EventOOC:       "OOC  ",
	EventIC:        "IC   ",
	EventJudge:     "JUD  ",
	EventDebug:     "DEBUG",
	EventFail:      "FAIL ",
}

// MakeRooms creates a list of rooms according to the room configuration.
func MakeRooms(charsConf *config.Characters, musicConf *config.Music) ([]*Room, error) {
	// TODO: warn about non-existant lists/adjancecies?
	roomConf, err := config.ReadRooms()
	if err != nil {
		return nil, fmt.Errorf("room: Couldn't read room config (%w).", err)
	}
	if len(roomConf.Confs) == 0 {
		return nil, fmt.Errorf("room: Empty room list.")
	}

	var rooms []*Room
	for i, conf := range roomConf.Confs {
		// Read characters.
		var chars []*char
		charLists := findCharLists(charsConf, conf.CharLists)
		for _, l := range charLists {
			for _, c := range l.Characters {
				chars = append(chars, &char{c, false})
			}
		}
		// Read music.
		var music []MusicCategory
		musicCats := findMusicCategories(musicConf, conf.SongCategories)
		for _, cat := range musicCats {
			music = append(music, MusicCategory(*cat))
		}

		var logOuts []string
		for _, mtd := range conf.LogMethods {
			switch mtd {
			case "terminal":
				logOuts = append(logOuts, "stdout")
			case "file":
				logOuts = append(logOuts,
					fmt.Sprintf("log/room/%v.log", strings.ReplaceAll(strings.ToLower(conf.Name), " ", "_")))
			}
		}

		lvl := logger.LevelInfo
		if conf.DebugLog {
			lvl = logger.LevelDebug
		}

		rooms = append(rooms, &Room{
			id:           i,
			name:         conf.Name,
			desc:         conf.DefaultDesc,
			chars:        chars,
			music:        music,
			sides:        conf.Sides,
			blankposting: conf.AllowBlankpost,
			iniswapping:  conf.AllowIniswap,
			shouting:     conf.AllowShouting,
			immediate:    conf.ForceImmediate,
			bg:           conf.DefaultBg,
			lockBg:       conf.LockBg,
			song:         packets.SongStop, // the canonical "stop" song for AO
			ambiance:     conf.DefaultAmbiance,
			status:       StatusIdle,
			lock:         LockFree,
			invited:      make(map[int]struct{}),
			// TODO: log to files
			logger: logger.NewLoggerOutputs(lvl, roomFormatter(i, conf.Name), logOuts...),
		})
	}

	// Configure adjacencies.
	for i, conf := range roomConf.Confs {
		// We check adjancecies for the i-th room.
		adjNames := conf.AdjacentRooms
		adjRooms := findRooms(rooms, adjNames)
		rooms[i].adjacent = adjRooms
		rooms[i].LogEventDebug(EventConfig, "Loaded configuration: %#v.", conf)
		rooms[i].LogEventDebug(EventConfig, "Current settings: %#v", rooms[i])
	}

	return rooms, nil
}

// Logs an event occurring in the room.
func (r *Room) LogEvent(event Event, format string, a ...any) {
	r.logger.Infof(" %v %v", eventToString[event], fmt.Sprintf(format, a...))
}

// Logs an event occurring in the room at debug level.
func (r *Room) LogEventDebug(event Event, format string, a ...any) {
	r.logger.Debugf(" %v %v", eventToString[event], fmt.Sprintf(format, a...))
}

// Attempts to enter a new user into the room. If unable, returns `false`.
// A CID of -1 (spectator) will bypass the check for available CIDs, and will always
// succeed.
// This doesn't check for locks or anything like that, that needs to be done externally.
func (r *Room) Enter(cid int, uid int) (ok bool) {
	r.mu.Lock()
	if cid == SpectatorCID {
		goto enter
	}
	if cid >= len(r.chars) || cid < 0 {
		r.LogEventDebug(EventFail, "UID %v tried joining with illegal CID (%v).", uid, cid)
		r.mu.Unlock()
		return false
	} else if r.chars[cid].taken {
		r.mu.Unlock() // Unlock so we can use GetNameByCID
		r.LogEventDebug(EventFail, "UID %v tried joining as %v (CID: %v), but this character is taken.",
			uid, r.GetNameByCID(cid), cid)
		return false
	}
	r.chars[cid].taken = true

enter:
	r.users = append(r.users, &user{charID: cid, userID: uid})
	r.mu.Unlock()
	r.LogEvent(EventEnter, "%v (CID: %v, UID: %v) entered.", r.GetNameByCID(cid), cid, uid)
	return true
}

// Removes a user from the room.
func (r *Room) Leave(uid int) {
	r.mu.Lock()

	u := r.getUser(uid)
	if u.userID == invalidUID {
		return
	}
	if u.charID != SpectatorCID {
		// shouldn't need an out-of-bounds check
		r.chars[u.charID].taken = false
	}
	r.mu.Unlock() // Unlock so we can get char name.
	r.LogEvent(EventExit, "%v (CID: %v, UID: %v) left.", r.GetNameByCID(u.charID), u.charID, u.userID)
	r.mu.Lock()
	r.removeUser(u.userID)
	r.mu.Unlock()
}

// Gets a character's name in the room's list by CID. If the CID is out of bounds,
// returns an empty string.
func (r *Room) GetNameByCID(cid int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cid == SpectatorCID {
		return "Spectator"
	}
	if cid < 0 || cid > len(r.chars) {
		return ""
	}
	return r.chars[cid].name
}

// Gets a character's CID in the room's list by their name. If the character is not found,
// `ok` will be `false`.
func (r *Room) GetCIDByName(name string) (cid int, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == "Spectator" {
		return SpectatorCID, true
	}
	for cid, c := range r.chars {
		if name == c.name {
			return cid, true
		}
	}
	return SpectatorCID, false
}

// Attempts a char change.
func (r *Room) ChangeChar(uid int, to int) (ok bool) {
	r.mu.Lock()

	usr := r.getUser(uid)
	from := usr.charID
	if from == to {
		r.mu.Unlock()
		return true
	}

	if to == SpectatorCID {
		goto change
	}

	if to < 0 || to >= len(r.chars) {
		r.mu.Unlock()
		r.LogEventDebug(EventFail, "%v (CID: %v, UID: %v) tried changing to illegal CID (%v).",
			r.GetNameByCID(from), from, uid, to)
		return false
	} else if r.chars[to].taken {
		r.mu.Unlock()
		r.LogEventDebug(EventFail, "%v (CID: %v, UID: %v) tried changing to %v (CID %v), but this character is taken.",
			r.GetNameByCID(from), from, uid, r.GetNameByCID(to), to)
		return false
	}
	r.chars[to].taken = true

change:
	usr.charID = to
	if from != SpectatorCID {
		r.chars[from].taken = false
	}
	r.mu.Unlock()
	r.LogEvent(EventCharacter, "%v (CID: %v, UID: %v) changed to %v (CID: %v).",
		r.GetNameByCID(from), from, uid, r.GetNameByCID(to), to)
	return true
}

// Returns the ID of the room.
func (r *Room) ID() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.id
}

// Returns the name of the room.
func (r *Room) Name() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.name
}

// Returns the description of the room.
func (r *Room) Desc() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.desc
}

// Returns the background of the room.
func (r *Room) Background() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bg
}

// Returns the current song in the room.
func (r *Room) Song() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.song
}

// Sets the current song in the room.
func (r *Room) SetSong(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.song = s
}

// Returns the CID of the last speaker.
func (r *Room) LastSpeaker() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastSpeaker
}

// Sets the CID of the last speaker.
func (r *Room) SetLastSpeaker(cid int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastSpeaker = cid
}

// Returns whether blankposts are allowed.
func (r *Room) AllowBlankpost() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.blankposting
}

// Returns whether iniswapping is allowed.
func (r *Room) AllowIniswapping() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.iniswapping
}

// Returns whether shouts are allowed.
func (r *Room) AllowShouting() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shouting
}

// Returns whether preanims are played immediately.
func (r *Room) ForceImmediate() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.immediate
}

// Returns the name of the track for the room's ambiance.
func (r *Room) Ambiance() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ambiance
}

// Sets the ambiance in the room.
func (r *Room) SetAmbiance(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ambiance = s
}

// Returns the list of adjacent rooms.
func (r *Room) Adjacent() []*Room {
	r.mu.Lock()
	defer r.mu.Unlock()
	rooms := make([]*Room, len(r.adjacent))
	copy(rooms, r.adjacent)
	return rooms
}

// Returns the list of visible rooms (adjacent rooms, and the room itself).
func (r *Room) Visible() []*Room {
	adj := r.Adjacent()
	adj = append([]*Room{r}, adj...)
	return adj
}

// Returns the list of names of visible rooms (adjacent rooms, and the room itself).
func (r *Room) VisibleNames() []string {
	vis := r.Visible()
	names := make([]string, len(vis))
	for i, v := range vis {
		names[i] = v.Name()
	}
	return names
}

// Returns all the UIDs in the room.
func (r *Room) UIDs() []int {
	r.mu.Lock()
	r.mu.Unlock()
	uids := make([]int, len(r.users))
	for i, u := range r.users {
		uids[i] = u.userID
	}
	return uids
}

// Returns the number of players in the room.
func (r *Room) PlayerCount() int {
	r.mu.Lock()
	r.mu.Unlock()
	return len(r.users)
}

// Returns the names of the characters in the room.
func (r *Room) Chars() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	list := make([]string, len(r.chars))
	for i, c := range r.chars {
		list[i] = c.name
	}
	return list
}

// Returns the length of the character list in the room.
func (r *Room) CharsLen() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.chars)
}

// Returns the music list (in AO format, i.e. categories and songs in the same list).
func (r *Room) MusicList() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var list []string
	for _, cat := range r.music {
		list = append(list, cat.Name)
		for _, s := range cat.Songs {
			list = append(list, string(s))
		}
	}
	return list
}

// Returns a copy of the music list as list of categories.
func (r *Room) Music() []MusicCategory {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := make([]MusicCategory, len(r.music))
	copy(list, r.music)
	return list
}

// Returns the length of the category list in the room.
func (r *Room) CategoriesLen() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.music)
}

// Returns the length of the music list in the room.
func (r *Room) MusicLen() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	for _, c := range r.music {
		for range c.Songs {
			count++
		}
	}
	return count
}

// Returns a copy of the room's side list.
func (r *Room) Sides() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	sides := make([]string, len(r.sides))
	copy(sides, r.sides)
	return sides
}

// Returns the room's status.
func (r *Room) Status() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return statusToString[r.status]
}

// Sets the room's status.
func (r *Room) SetStatus(s Status) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = s
}

// Returns the room's lock state.
func (r *Room) LockState() LockState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lock
}

// Returns the room's lock state as a string (as in AO).
func (r *Room) LockString() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return lockToString[r.lock]
}

// Sets the room's lock state.
func (r *Room) SetLockState(s LockState) {
	r.mu.Lock()
	defer r.mu.Lock()
	r.lock = s
}

// Returns a list of invited UIDs.
func (r *Room) Invited() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	l := make([]int, len(r.invited))
	for u := range r.invited {
		l = append(l, u)
	}
	return l
}

// Returns whether the passed UID is invited or not.
func (r *Room) IsInvited(uid int) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for u := range r.invited {
		if u == uid {
			return true
		}
	}
	return false
}

// Adds the passed UID to the invite list.
func (r *Room) Invite(uid int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.invited[uid] = struct{}{}
}

// Removes the passed UID to the invite list.
func (r *Room) Uninvite(uid int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.invited, uid)
}

// Clears the invite list.
func (r *Room) ClearInvites() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.invited)
}

// Returns the list of taken CIDs.
func (r *Room) Taken() []bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	taken := make([]bool, len(r.chars))
	for cid, char := range r.chars {
		taken[cid] = char.taken
	}
	return taken
}

// Returns a list of taken CIDs as strings (for the CharsCheck AO packet).
// Cursed, yes.
func (r *Room) TakenList() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var takenList []string
	for _, c := range r.chars {
		if c.taken {
			takenList = append(takenList, "-1")
		} else {
			takenList = append(takenList, "0")
		}
	}
	return takenList
}

// Private methods don't lock the room's mutex. That is to be done in the
// public methods that call them.

// Gets a user (CID-UID pair) by their UID.
func (r *Room) getUser(uid int) *user {
	for _, u := range r.users {
		if u.userID == uid {
			return u
		}
	}
	// shouldn't happen, probably
	r.logger.Errorf("Tried to get non-existant UID (%v)! This shouldn't happen. Warn the developer!", uid)
	return &user{SpectatorCID, invalidUID}
}

func (r *Room) removeUser(uid int) {
	for i, u := range r.users {
		if u.userID == uid {
			// Order doesn't matter, so we can do this.
			r.users[i] = r.users[len(r.users)-1]
			r.users = r.users[:len(r.users)-1]
		}
	}
}
