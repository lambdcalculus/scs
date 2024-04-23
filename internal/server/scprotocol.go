package server

import (
	"encoding/json"

	"github.com/lambdcalculus/scs/internal/logger"
	"github.com/lambdcalculus/scs/internal/client"
	"github.com/lambdcalculus/scs/pkg/packets"
)

type handleFuncSC func(srv *SCServer, c *client.Client, data []byte)

var handlerMapSC = map[string]handleFuncSC{
	"hello": (*SCServer).handleHello,
}

func (srv *SCServer) handlePacketSC(c *client.Client, pkt packets.PacketSC) {
	if handler := handlerMapSC[pkt.Header]; handler != nil {
        // There may be a better way to do this. In total, the data is unmarshaled, remarshaled and unmarshaled again.
        // Considering Go doesn't let us do much with pkt.Data since it's just an interface{}, 
        // I don't think there is a better way until you start using reflection.

		// It was unmarshaled succesfully before, so we don't have to check the marshaling.
		data, _ := json.Marshal(pkt.Data)
		handler(srv, c, data)
	}
}

func (srv *SCServer) handleHello(c *client.Client, data []byte) {
	var hello packets.DataHelloClient
	err := json.Unmarshal(data, &hello)
	if err != nil {
		logger.Debugf("Bad 'hello' from %v: %s", c.Addr(), data)
		return
	}

	// c.ident = hello.Ident

    taken := srv.rooms[0].Taken()
    // TODO: consider pre-allocating instead of appending dynamically?
    var takenList []string
    for i, char := range srv.rooms[0].Chars() {
        if taken[i] {
            takenList = append(takenList, char)
        }
    }
	c.WriteSC("CHARLIST", srv.rooms[0].Chars())
    c.WriteSC("CHARLISTTAKEN", taken)

    // TODO: better way to do this?
    cats := make([]packets.MusicCategory, srv.rooms[0].CategoriesLen())
    for i, c := range srv.rooms[0].Music() {
        songs := make([]string, len(c.Songs))
        for j, s := range c.Songs {
            songs[j] = string(s)
        }
        cats[i] = packets.MusicCategory{
            Name: c.Name,
            Songs: songs,
        }
    }
    c.WriteSC("MUSICLIST", cats)
}
