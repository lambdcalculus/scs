// Package `perms` handles special permissions.
package perms

import (
	"fmt"

	"github.com/lambdcalculus/scs/internal/config"
)

// Permissions are given by a 32-bit bitmask.
type Mask uint32

const (
	None Mask = 0

	// Moderator stuff.

	// Permission to see IPIDs.
	SeeIPIDs Mask = 1 << iota
	// Permission to hear mod calls.
	HearModCalls
	// Permission to mute users.
	Mute
	// Permission to kick users.
	Kick
	// Permission to ban users.
	Ban
	// Permission to bypass locks (e.g. room locks, background locks, etc.).
	BypassLocks

	// Room stuff.

	// Permission to change the room's status.
	Status
	// Permission to change the room's lock.
	Lock
	// Permission to change the room's description.
	Description
	// Permission to change the room's background (does not bypass background lock).
	Background
	// Permission to change the room's ambiance track (does not bypass ambiance lock).
	Ambiance

	All Mask = 0xffffffff
)

type Role struct {
	Name  string
	Perms Mask
}

// Checks if the given role has the passed permissions.
func (r *Role) Check(p Mask) bool {
	return r.Perms&p == p
}

var stringToPerm = map[string]Mask{
	"hear_modcall": HearModCalls,
	"see_ipids":    SeeIPIDs,
	"mute":         Mute,
	"kick":         Kick,
	"ban":          Ban,
	"bypass_locks": BypassLocks,
	"status":       Status,
	"description":  Description,
	"background":   Background,
	"ambiance":     Ambiance,
	"all":          All,
}

// Makes a list of roles out of the roles configuration.
func MakeRoles() ([]Role, error) {
	confs, err := config.ReadRoles()
	if err != nil {
		return nil, fmt.Errorf("perms: Couldn't read roles (%w).", err)
	}
	roles := make([]Role, len(confs.Confs))
	for i, conf := range confs.Confs {
		perms := None
		for _, s := range conf.Permissions {
			perms |= stringToPerm[s]
		}
		roles[i] = Role{
			Name:  conf.Name,
			Perms: perms,
		}
	}
	return roles, nil
}
