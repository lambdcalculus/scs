package config

import (
	"fmt"
	"os"
	"path"

	"github.com/BurntSushi/toml"
)

type Server struct {
	Name       string `toml:"name"`
	Username   string `toml:"server_username"`
	Desc       string `toml:"description"`
	MaxPlayers int    `toml:"max_players"`
	PortWS     int    `toml:"ws_port"`
	PortTCP    int    `toml:"legacy_port"`
	AssetURL   string `toml:"asset_url"`
    //TODO: AllowAO bool `toml:"allow_ao"`

    // these seem more appropriate for a different section?
	MaxMsgSize  int `toml:"max_msg_size"`
	MaxNameSize int `toml:"max_name_size"`
}

type Room struct {
	Name            string `toml:"name"`
	DefaultBg       string `toml:"background"`
	DefaultDesc     string `toml:"description"`
	DefaultAmbiance string `toml:"ambiance"`

	AdjacentRooms  []string `toml:"adjacent_rooms"`
	CharLists      []string `toml:"character_lists"`
	SongCategories []string `toml:"song_categories"`
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

// Attempts to read server configuration. Returns zero [Server] if it fails.
func ReadServer() (Server, error) {
	execDir, err := getExecDir()
	if err != nil {
		return Server{}, fmt.Errorf("config: Couldn't find executable location (%w). Can't read configs.", err)
	}
	configDir := execDir + "/config"

	srvConfig, err := readConfig(configDir)
	if err != nil {
		return Server{}, fmt.Errorf("config: Couldn't read main config (%w).", err)
	}

	return srvConfig, nil
}

// Attempts to read room settings. Returns the zero [RoomList] and an error if it fails.
func ReadRooms() (RoomList, error) {
	execDir, err := getExecDir()
	if err != nil {
		return RoomList{}, fmt.Errorf("config: Couldn't find executable location (%w). Can't read configs.", err)
	}
	configDir := execDir + "/config"

	var list RoomList
	if _, err = toml.DecodeFile(configDir+"/room.toml", &list); err != nil {
		return RoomList{}, fmt.Errorf("config: Couldn't read rooms (%w).", err)
	}
	return list, nil
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

func readConfig(configDir string) (Server, error) {
	srvConfig := Server{}
	_, err := toml.DecodeFile(configDir+"/config.toml", &srvConfig)
	if err != nil {
		return Server{}, err
	}

	return srvConfig, nil
}

func getExecDir() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return path.Dir(execPath), nil

}
