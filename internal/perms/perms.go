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

	SeeIPIDs     Mask = 1 << iota // Permission to see IPIDs.
	HearModCalls                  // Permission to hear mod calls.
	Mute                          // Permission to mute users.
	Kick                          // Permission to kick users.
	Ban                           // Permission to ban users.
	Unban                         // Permission to unban users.
	BypassLocks                   // Permission to bypass locks (e.g. room locks, background locks, etc.).

	// Room stuff.

	Status      // Permission to change the room's status.
	Lock        // Permission to change the room's lock.
	Description // Permission to change the room's description.
	Background  // Permission to change the room's background (necessary when there is a background lock).
	Ambiance    // Permission to change the room's ambiance track (necessary when there is an ambiance lock).

	// Admin stuff.

	ModifyDatabase // Permission to use commands that alter the database directly.
	ReservedNames  // Permission to bypass the server's reserved names.

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
	"hear_modcall":   HearModCalls,
	"see_ipids":      SeeIPIDs,
	"mute":           Mute,
	"kick":           Kick,
	"ban":            Ban,
	"unban":          Unban,
	"bypass_locks":   BypassLocks,
	"status":         Status,
	"lock":           Lock,
	"description":    Description,
	"background":     Background,
	"ambiance":       Ambiance,
	"mod_database":   ModifyDatabase,
	"reserved_names": ReservedNames,
	"all":            All,
}

// Makes a list of roles out of a roles configuration.
func MakeRoles(confs *config.Roles) ([]Role, error) {
	confs, err := config.ReadRoles()
	if err != nil {
		return nil, fmt.Errorf("perms: Couldn't read roles config (%w)", err)
	}
	roles := make([]Role, len(confs.Confs))
	for i, conf := range confs.Confs {
		perms := None
		for _, s := range conf.Permissions {
			if len(s) == 0 {
				return nil, fmt.Errorf("perms: Empty permission string in role %s", conf.Name)
			}
			if s[0] == '^' {
				perm, ok := stringToPerm[s[1:]]
				if !ok {
					return nil, fmt.Errorf("perms: Unknown permission: %s", s[1:])
				}
				perms &= ^perm
				continue
			}
			perm, ok := stringToPerm[s]
			if !ok {
				return nil, fmt.Errorf("perms: Unknown permission: %s", s)
			}
			perms |= perm
		}
		roles[i] = Role{
			Name:  conf.Name,
			Perms: perms,
		}
	}
	return roles, nil
}

// Checks if the permissions in `p` are a (non-strict) subset of the ones in `q`.
func (p Mask) Subset(q Mask) bool {
    // time for some boolean logic
    // "p implies q" is equivalent to "q or not p", therefore...
    return q | ^p == All
}
