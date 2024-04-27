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
	DefaultBg       string `toml:"background"`
	DefaultDesc     string `toml:"description"`
	DefaultAmbiance string `toml:"ambiance"`

	AdjacentRooms  []string `toml:"adjacent_rooms"`
	CharLists      []string `toml:"character_lists"`
	SongCategories []string `toml:"song_categories"`

	// TODO: add buffered logging
	LogMethods []string `toml:"log_methods"`
	DebugLog   bool     `toml:"log_debug"`
}

func RoomDefault() *Room {
	return &Room{
		Name:           "Unknown",
		CharLists:      []string{"all"},
		SongCategories: []string{"all"},
		AdjacentRooms:  []string{},
		LogMethods:     []string{"file"},
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

// Attempts to read server configuration. Returns default server settings if it fails.
func ReadServer() (*Server, error) {
	execDir, err := getExecDir()
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

// Attempts to read room settings. Returns the zero [RoomList] and an error if it fails.
func ReadRooms() (*RoomList, error) {
	execDir, err := getExecDir()
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

// Attempts to read character settings. Returns the zero [CharList] and an error if it fails.
func ReadCharacters() (Characters, error) {
	execDir, err := getExecDir()
	if err != nil {
		return Characters{}, fmt.Errorf("config: Couldn't find executable location (%w). Can't read configs.", err)
	}
	configDir := execDir + "/config"

	var list Characters
	if _, err = toml.DecodeFile(configDir+"/characters.toml", &list); err != nil {
		return Characters{}, fmt.Errorf("config: Couldn't read rooms (%w).", err)
	}
	return list, nil
}

// Attempts to read music settings. Returns the zero [Music] and an error if it fails.
func ReadMusic() (Music, error) {
	execDir, err := getExecDir()
	if err != nil {
		return Music{}, fmt.Errorf("config: Couldn't find executable location (%w). Can't read configs.", err)
	}
	configDir := execDir + "/config"

	var conf Music
	if _, err = toml.DecodeFile(configDir+"/music.toml", &conf); err != nil {
		return Music{}, fmt.Errorf("config: Couldn't read rooms (%w).", err)
	}
	return conf, nil
}

func getExecDir() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return path.Dir(execPath), nil

}
