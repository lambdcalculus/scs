package config

import (
	"fmt"
	"os"
	"path"

	"github.com/BurntSushi/toml"
	"github.com/lambdcalculus/scs/internal/logger"
)

type Server struct {
	Name       string `toml:"name"`
	Username   string `toml:"server_username"`
	Desc       string `toml:"description"`
	MaxPlayers int    `toml:"max_players"`
	PortWS     int    `toml:"ws_port"`
	PortTCP    int    `toml:"legacy_port"`
	PortRPC    int    `toml:"rpc_port"`
	AllowAO    bool   `toml:"allow_ao"`
	AssetURL   string `toml:"asset_url"`
	//TODO: AllowAO bool `toml:"allow_ao"`

	// these seem more appropriate for a different section?
	MaxMsgSize  int `toml:"max_msg_size"`
	MaxNameSize int `toml:"max_name_size"`

	LevelString string `toml:"log_level"`
}

func ServerDefault() *Server {
	return &Server{
		Name:        "Unnamed Server",
		Username:    "SCS",
		Desc:        "An unconfigured SpriteChat server.",
		MaxPlayers:  100,
		PortWS:      8080,
		PortTCP:     8081,
		PortRPC:     8082,
		AssetURL:    "",
		MaxMsgSize:  150,
		MaxNameSize: 20,
		LevelString: "info",
	}
}

var StringToLevel = map[string]logger.LogLevel{
	"trace": logger.LevelTrace,
	"debug": logger.LevelDebug,
	"info":  logger.LevelInfo,
	"warn":  logger.LevelWarning,
	"error": logger.LevelError,
	"fatal": logger.LevelFatal,
}

type Room struct {
	Name            string `toml:"name"`
	DefaultDesc     string `toml:"description"`
	DefaultBg       string `toml:"background"`
	LockBg          bool   `toml:"lock_background"`
	DefaultAmbiance string `toml:"ambiance"`
	LockAmbiance    bool   `toml:"lock_ambiance"`

	AdjacentRooms  []string `toml:"adjacent_rooms"`
	CharLists      []string `toml:"character_lists"`
	SongCategories []string `toml:"song_categories"`
	Sides          []string `toml:"side_list"`

	AllowBlankpost bool `toml:"allow_blankpost"`
	AllowShouting  bool `toml:"allow_shouting"`
	AllowIniswap   bool `toml:"allow_iniswap"`
	ForceImmediate bool `toml:"force_immediate"`

	// TODO: add buffered logging
	LogMethods []string `toml:"log_methods"`
	DebugLog   bool     `toml:"log_debug"`
}

func RoomDefault() *Room {
	return &Room{
		Name:            "Unknown",
		DefaultAmbiance: "~stop.mp3",
		CharLists:       []string{"all"},
		SongCategories:  []string{"all"},
		Sides:           []string{"wit", "def", "pro", "jud", "hld", "hlp"},
		AdjacentRooms:   []string{},
		LogMethods:      []string{"file"},
		AllowBlankpost:  true,
		AllowShouting:   true,
		AllowIniswap:    true,
		ForceImmediate:  false,
	}
}

type RoomList struct {
	Confs []Room `toml:"room"`
}

// Right now we are using strings for songs and characters, but they could
// be more complicated structures with more metadata later.
type (
	Character string
	Song      string //TODO: song aliases
)

type CharList struct {
	Name       string   `toml:"name"`
	Characters []string `toml:"characters"`
}

type Characters struct {
	Lists []CharList `toml:"list"`
}

type SongCategory struct {
	Name  string `toml:"name"`
	Songs []Song `toml:"songs"`
}

type Music struct {
	Categories []SongCategory `toml:"category"`
}

type Role struct {
	Name        string   `toml:"name"`
	Permissions []string `toml:"permissions"`
}

type Roles struct {
	Confs []Role `toml:"role"`
}

// Attempts to read server configuration. Returns default server settings if it fails.
func ReadServer() (*Server, error) {
	execDir, err := ExecDir()
	if err != nil {
		return ServerDefault(), fmt.Errorf("config: Couldn't find executable location (%w). Can't read configs.", err)
	}
	configDir := execDir + "/config"

	srvConfig := ServerDefault()
	if _, err := toml.DecodeFile(configDir+"/config.toml", srvConfig); err != nil {
		return srvConfig, fmt.Errorf("config: Couldn't read server config (%w).", err)
	}

	return srvConfig, nil
}

// Currently, to enforce the default settings for rooms, we're reading the room list
// twice. This isn't awful, but maybe we can do better. So, TODO: do better.

// Attempts to read room settings. Returns nil [RoomList] and an error if it fails.
func ReadRooms() (*RoomList, error) {
	execDir, err := ExecDir()
	if err != nil {
		return nil, fmt.Errorf("config: Couldn't find executable location (%w). Can't read configs.", err)
	}
	configDir := execDir + "/config"

	num, err := countRooms(configDir)
	if err != nil {
		return nil, fmt.Errorf("config: Couldn't read rooms (%w).", err)
	}

	list := RoomList{Confs: make([]Room, num)}
	for i := range list.Confs {
		list.Confs[i] = *RoomDefault()
	}
	if _, err = toml.DecodeFile(configDir+"/room.toml", &list); err != nil {
		return nil, fmt.Errorf("config: Couldn't read rooms (%w).", err)
	}
	return &list, nil
}

// Counts the amount of rooms in the settings, if they can be found.
func countRooms(configDir string) (int, error) {
	var list RoomList
	if _, err := toml.DecodeFile(configDir+"/room.toml", &list); err != nil {
		return 0, err
	}
	return len(list.Confs), nil
}

// Attempts to read character settings. Returns nil [CharList] and an error if it fails.
func ReadCharacters() (*Characters, error) {
	execDir, err := ExecDir()
	if err != nil {
		return nil, fmt.Errorf("config: Couldn't find executable location (%w). Can't read configs.", err)
	}
	configDir := execDir + "/config"

	var list Characters
	if _, err = toml.DecodeFile(configDir+"/characters.toml", &list); err != nil {
		return nil, fmt.Errorf("config: Couldn't read characters (%w).", err)
	}
	return &list, nil
}

// Attempts to read music settings. Returns the nil [Music] and an error if it fails.
func ReadMusic() (*Music, error) {
	execDir, err := ExecDir()
	if err != nil {
		return nil, fmt.Errorf("config: Couldn't find executable location (%w). Can't read configs.", err)
	}
	configDir := execDir + "/config"

	var conf Music
	if _, err = toml.DecodeFile(configDir+"/music.toml", &conf); err != nil {
		return nil, fmt.Errorf("config: Couldn't read music (%w).", err)
	}
	return &conf, nil
}

func ReadRoles() (*Roles, error) {
	execDir, err := ExecDir()
	if err != nil {
		return nil, fmt.Errorf("config: Couldn't find executable location (%w). Can't read configs.", err)
	}
	configDir := execDir + "/config"

	var list Roles
	if _, err = toml.DecodeFile(configDir+"/roles.toml", &list); err != nil {
		return nil, fmt.Errorf("config: Couldn't read roles (%w).", err)
	}
	return &list, nil
}

// Returns the absolute path to the executable's directory, if it doesn't fail.
func ExecDir() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return path.Dir(execPath), nil

}
