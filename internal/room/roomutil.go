package room

import (
	"fmt"
	"time"

	"github.com/lambdcalculus/scs/internal/config"
	"github.com/lambdcalculus/scs/internal/logger"
)

// Returns the charlists in the configuration that correspond to the list of names in linear time.
// If `"all"` is in the list, return all of the charlists.
// TODO: this makes it so you can't "order" the character lists. Change?
func findCharLists(conf *config.Characters, names []string) []*config.CharList {
	set := make(map[string]struct{})
	for _, n := range names {
		set[n] = struct{}{}
	}

	var lists []*config.CharList
	if _, ok := set["all"]; ok {
		for _, l := range conf.Lists {
			lists = append(lists, &l)
		}
		return lists
	}
	for _, l := range conf.Lists {
		if _, ok := set[l.Name]; ok {
			lists = append(lists, &l)
		}
	}
	return lists
}

// Returns the music categories in the configuration that correspond to the list of names in linear time.
// If `"all"` is in the list, return all of the categories.
func findMusicCategories(conf *config.Music, names []string) []*config.SongCategory {
	set := make(map[string]struct{})
	for _, n := range names {
		set[n] = struct{}{}
	}

	var cats []*config.SongCategory
	if _, ok := set["all"]; ok {
		for _, cat := range conf.Categories {
			cats = append(cats, &cat)
		}
		return cats
	}
	for _, cat := range conf.Categories {
		if _, ok := set[cat.Name]; ok {
			cats = append(cats, &cat)
		}
	}
	return cats
}

// Returns the rooms in the passed list that correspond to the list of names passed.
func findRooms(list []*Room, names []string) []*Room {
	set := make(map[string]struct{})
	for _, n := range names {
		set[n] = struct{}{}
	}

	var rooms []*Room
	if _, ok := set["all"]; ok {
		for _, r := range list {
			rooms = append(rooms, r)
		}
		return list
	}
	for _, r := range list {
		if _, ok := set[r.Name()]; ok {
			rooms = append(rooms, r)
		}
	}
	return rooms
}

// Returns a [logger.FormatFunc] that matches the given name and id.
func roomFormatter(id int, name string) logger.FormatFunc {
	return func(msg string, lvl logger.LogLevel) string {
		// Get time right away.
		logTime := time.Now().Format(time.RubyDate)

		// Don't duplicate newlines.
		if len(msg) > 1 && msg[len(msg)-1] == '\n' {
			msg = msg[:len(msg)-2]
		}

		if lvl >= logger.LevelError {
			return fmt.Sprintf("[ERROR]\t[(Room %v) %v]\t[%v]: %v\n", id, name, logTime, msg)
		}
		return fmt.Sprintf("[%v] (Room %v) %v : %v\n", name, id, logTime, msg)
	}
}
