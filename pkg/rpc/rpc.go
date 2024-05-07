// Package `rpc` exports methods to interface with an RPC server.
//
// This separation allows RPC clients to not require importing the `server`
// package, which makes them a lot lighter.
//
// The "Impl" variables are to be used by the server for the internal implementations
// of each RPC opeartion.
package rpc

import (
    "fmt"
    "time"
    "net/rpc"
    "net/http"
)

// The receivers for the exported RPC methods.
type (
    DB int
    // TODO: commands for AO server? e.g. say, kick, etc.
)

// Arguments for the AddAuth operation.
type AddAuthArgs struct {
	Username string
	Password string
	Role     string
}

// Arguments for the RmAuth operation.
type RmAuthArgs struct {
	Username string
}

// These define the internal implementation of each operation.
// They only need to be set by the server, RPC clients can ignore this.
var (
	AddAuthImpl = func(args *AddAuthArgs, reply *int) error { return nil }
	RmAuthImpl  = func(args *RmAuthArgs, reply *int) error { return nil }
)

// Returns an HTTP server that serves RPC in the passed port.
// The "Impl" variables should be used to configure its operations
// before running the server.
// If there is an issue setting up the server, returns an error.
func NewServer(port int) (*http.Server, error) {
    r := new(DB)
    s := rpc.NewServer()
    if err := s.Register(r); err != nil {
        return nil, err
    }

	return &http.Server{
        Addr:           fmt.Sprintf("localhost:%v", port),
		Handler:        s,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}, nil

}

// Adds an user to the auth table in the database.
func (*DB) AddAuth(args *AddAuthArgs, reply *int) error {
    return AddAuthImpl(args, reply)
}

// Removes an user from the auth table in the database.
func (*DB) RmAuth(args *RmAuthArgs, reply *int) error {
    return RmAuthImpl(args, reply)
}
