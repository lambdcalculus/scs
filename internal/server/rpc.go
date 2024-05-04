package server

import (
	"fmt"
	"net/http"
	"net/rpc"
	"time"
)

// TODO: try to find a way to remove the necessity to import the `server`
// package in the RPC client. Though I think this import is intended to
// be necessary, with the way the Go RPC API works.

// Listens for local RCP connections, for usage with serverctl.
func (srv *SCServer) listenRPC() {
    s := rpc.NewServer()
    if err := s.Register(srv); err != nil {
        srv.logger.Errorf("Couldn't register RCP commands (%s).", err)
    }

	rpcServer := &http.Server{
        Addr:           fmt.Sprintf("localhost:%v", srv.config.PortRPC),
		Handler:        s,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

    srv.logger.Infof("Listening RPC on port %v.", srv.config.PortRPC)
    srv.logger.Errorf("Stopped serving RPC (%v).", rpcServer.ListenAndServe())
}

// Arguments for the AddAuth operation.
type AddAuthArgs struct {
    Username string
    Password string
    Role     string
}

// Adds an user to the auth table in the database.
func (srv *SCServer) AddAuth(args *AddAuthArgs, reply *int) error {
    if err := srv.db.AddAuth(args.Username, args.Password, args.Role); err != nil {
        *reply = 1
        return err
    }
    *reply = 0
    return nil
}

// Arguments for the RmAuth operation.
type RmAuthArgs struct {
    Username string
}

// Removes an user from the auth table in the database.
func (srv *SCServer) RmAuth(args *RmAuthArgs, reply *int) error {
    if err := srv.db.RemoveAuth(args.Username); err != nil {
        *reply = 1
        return err
    }
    *reply = 0
    return nil
}
