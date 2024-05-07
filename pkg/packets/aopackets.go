package packets

import "strings"

type PacketAO struct {
    Header string
    Contents []string
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

// Bar selection for the HP packet.
type BarSelect int

const (
    BarDef BarSelect = 1
    BarPro BarSelect = 2
)

// Bar HP for the HP packet.
type BarHP int
const (
    BarMin BarHP = 0
    BarMax BarHP = 10
)

// Makes an AO packet from raw bytes.
func MakeAOPacket(raw []byte) PacketAO {
    sb := strings.Builder{}
    sb.Write(raw)
    parts := strings.Split(sb.String(), "#")

    if len(parts) < 2 {
        return PacketAO{}
    }
    return PacketAO{
        Header: parts[0],
        Contents: parts[1:len(parts)-1],
    }
}

// Because of the way AO packets work, we can't have '%', '&', '#' or "$" where they shouldn't be.
// So they are encoded as '<percent>', '<and>', '<num>' and '<dollar>'.

// Encodes an AO packet.
func (p *PacketAO) Encode() {
    for i, s := range p.Contents {
        p.Contents[i] = encode(s)
    }
}

// Decodes an AO packet.
func (p *PacketAO) Decode() {
    for i, s := range p.Contents {
        p.Contents[i] = decode(s)
    }
}

func encode(s string) string {
	return strings.NewReplacer("%", "<percent>",
		"&", "<and>",
		"#", "<num>",
		"$", "<dollar>").Replace(s)
}

func decode(s string) string {
	return strings.NewReplacer("<percent>", "%",
		"<and>", "&",
		"<num>", "#",
		"<dollar>", "$").Replace(s)
}
