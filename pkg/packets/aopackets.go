package packets

import "strings"

type PacketAO struct {
    Header string
    Contents []string
}

func MakeAOPacket(raw []byte) PacketAO {
    parts := strings.Split(string(raw[:]), "#")
    if len(parts) < 2 {
        return PacketAO{}
    }

    return PacketAO{
        Header: parts[0],
        Contents: parts[1:len(parts)-1],
    }
}

// Area updates for the ARUP packet.
type AreaUpdate int

const (
	UpdatePlayer AreaUpdate = 1 << iota
	UpdateStatus
	UpdateManager
	UpdateLock

	UpdateAll AreaUpdate = 0b1111
)

// Song effects for the MC packet.
type SongEffect int

const (
    EffectFadeIn SongEffect = 1 << iota
    EffectFadeOut 
    EffectSync
)

// The canonical stop song for AO.
const SongStop string = "~stop.mp3"
