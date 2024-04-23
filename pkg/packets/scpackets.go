package packets

import "encoding/json"

type PacketSC struct {
	Header string      `json:"header"`
	Data   interface{} `json:"data"`
}

func MakeSCPacket(raw []byte) (PacketSC, error) {
	var p PacketSC
	if err := json.Unmarshal(raw, &p); err != nil {
		return PacketSC{}, err
	}
	return p, nil
}

// Client packets

type DataHelloClient struct {
	App     string `json:"application"`
	Version string `json:"version"`
	Ident   string `json:"identifier"`
}

// Server packets

type DataHelloServer struct {
	App      string   `json:"application"`
	Version  string   `json:"version"`
	Name     string   `json:"name"`
	Desc     string   `json:"description"`
	Players  int      `json:"playercount"`
	URL      string   `json:"url"`
	Packages []string `json:"packages"`
}

type DataCharList []string
type DataCharListTaken []string

type MusicCategory struct {
	Name  string   `json:"category"`
	Songs []string `json:"songs"`
}
type DataMusicList []MusicCategory
