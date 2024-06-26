package server

import (
	"github.com/lambdcalculus/scs/pkg/rpc"
)

// TODO: try to find a way to remove the necessity to import the `server`
// package in the RPC client. Though I think this import is intended to
// be necessary, with the way the Go RPC API works.

// Listens for local RCP connections, for usage with serverctl.
func (srv *SCServer) listenRPC() {
	s, err := rpc.NewServer(srv, srv.config.PortRPC)
	if err != nil {
		srv.logger.Errorf("Couldn't create RPC server (%s).", err)
		return
	}

	srv.logger.Infof("Listening RPC on port %v.", srv.config.PortRPC)
	srv.logger.Errorf("Stopped serving RPC (%v).", s.HTTP.ListenAndServe())
}

// Adds an user to the auth table in the database.
func (srv *SCServer) AddAuth(args *rpc.AddAuthArgs, reply *int) error {
	if err := srv.db.AddAuth(args.Username, args.Password, args.Role); err != nil {
		srv.logger.Infof("rpc: Failed AddAuth request. Arguments: %#v.", *args)
		*reply = 1
		return err
	}
	*reply = 0
	srv.logger.Infof("rpc: Successful AddAuth request. Arguments: %#v.", *args)
	return nil
}

// Removes an user from the auth table in the database.
func (srv *SCServer) RmAuth(args *rpc.RmAuthArgs, reply *int) error {
	if err := srv.db.RemoveAuth(args.Username); err != nil {
		srv.logger.Infof("rpc: Failed RmAuth request. Arguments: %#v.", *args)
		*reply = 1
		return err
	}
	srv.logger.Infof("rpc: Successful RmAuth request. Arguments: %#v.", *args)
	*reply = 0
	return nil
}
