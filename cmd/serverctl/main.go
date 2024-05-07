// serverctl implements an RPC client to manage the server.
package main

import (
	"fmt"
	"net/rpc"
	"os"
	"strconv"

	// using `t`` since we only require the RPC types
	t "github.com/lambdcalculus/scs/pkg/rpc"
	"github.com/lambdcalculus/scs/pkg/logger"
	"github.com/spf13/pflag"
)

type cmdHandler func(args []string)

type command struct {
	handler cmdHandler
	// flagset     *pflag.FlagSet // Unnecessary for now.
	args        int
	description string
	usage       string
}

// Unnecessary for now, none of these require flags.
// var (
// 	cmdHelp    *pflag.FlagSet
// 	cmdAddAuth *pflag.FlagSet
// 	cmdRmAuth  *pflag.FlagSet
// )

var commands map[string]command

// TODO: detect port from config automatically?
var rpcPort int

func init() {
	logger.SetLogger(logger.NewLoggerOutputs(logger.LevelInfo, logFormat, "stdout"))

	pflag.CommandLine.SetOutput(os.Stdout)
	pflag.CommandLine.Usage = printUsage

	// cmdHelp = pflag.NewFlagSet("help", pflag.ExitOnError)
	// cmdHelp.Usage = func() {
	//     fmt.Printf(
	//     "Usage of help:\n" +
	//     "    serverctl help [command]\n")
	// }

	// cmdAddAuth = pflag.NewFlagSet("add-auth", pflag.ExitOnError)
	// cmdAddAuth.Usage = func() {
	// fmt.Printf(
	// "Usage of add-auth:\n" +
	// "    serverctl -p [RPC port] add-auth [username] [password] [role]\n",
	// )
	// }

	commands = map[string]command{
		"help": {handleHelp, 0, "shows usage information about a command",
			"serverctl help [command]"},
		"add-auth": {handleAddAuth, 3, "adds an user to the auth table",
			"serverctl -p [RPC port] add-auth [username] [password] [role]"},
		"rm-auth": {handleRmAuth, 1, "removes an user from the auth table",
			"serverctl -p [RPC port] rm-auth [username]"},
	}

	pflag.IntVarP(&rpcPort, "port", "p", -1, "port used for RPC")
}

func main() {
	pflag.Parse()

	if len(pflag.Args()) < 1 {
		logger.Fatalf("No command given.")
		pflag.CommandLine.Usage()
		os.Exit(1)
	}

	cmdName := pflag.Args()[0]
	cmd, ok := commands[pflag.Args()[0]]
	if !ok {
		logger.Fatalf("Unknown command.")
		pflag.CommandLine.Usage()
		os.Exit(1)
	}

	var cmdArgs []string
	if len(pflag.Args()) <= 1 {
		cmdArgs = []string{}
	} else {
		cmdArgs = pflag.Args()[1:]
	}

	if len(cmdArgs) < cmd.args {
		logger.Fatalf("Not enough arguments for %v (need %v, got %v).", cmdName, cmd.args, len(cmdArgs))
		handleHelp([]string{cmdName})
		os.Exit(1)
	}
	cmd.handler(cmdArgs)
	os.Exit(0)

	// args := &server.AddAuthArgs{
	// 	Username: "lambdcalculus",
	// 	Password: "lol",
	// 	Role:     "Admin",
	// }
	// var reply int
	// err = client.Call("SCServer.AddAuth", args, &reply)
	// if err != nil {
	// 	logger.Fatalf("calling: %s", err)
	// }
	// fmt.Printf("Reply: %v", reply)
}

func handleHelp(args []string) {
	if len(args) < 1 {
		pflag.CommandLine.Usage()
		return
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Printf("help: command '%v' does not exist.\n", args[0])
		os.Exit(1)
	}
	fmt.Printf("Usage of %v:\n", args[0])
	fmt.Printf("    %v\n", cmd.usage)
}

func handleAddAuth(args []string) {
	client := dial()
	rpcArgs := &t.AddAuthArgs{
		Username: args[0],
		Password: args[1],
		Role:     args[2],
	}
	var reply int
	if err := client.Call("DB.AddAuth", rpcArgs, &reply); err != nil {
		logger.Errorf("add-auth: Failed (%s).", err)
		os.Exit(1)
	}
	fmt.Printf("add-auth: User '%v' with role '%v' added succesfully!\n", args[0], args[2])
}

func handleRmAuth(args []string) {
	client := dial()
	rpcArgs := &t.RmAuthArgs{
		Username: args[0],
	}
	var reply int
	if err := client.Call("DB.RmAuth", rpcArgs, &reply); err != nil {
		logger.Errorf("rm-auth: Failed (%s).", err)
		os.Exit(1)
	}
	fmt.Printf("rm-auth: User '%v' removed succesfully!\n", args[0])
}

func dial() *rpc.Client {
	if rpcPort <= 0 {
		logger.Fatalf("Port must be specified.")
		pflag.CommandLine.Usage()
		os.Exit(1)
	}

	client, err := rpc.DialHTTP("tcp", "localhost:"+strconv.Itoa(rpcPort))
	if err != nil {
		logger.Fatalf("Couldn't dial server (%s).", err)
		os.Exit(1)
	}
	return client
}

func printUsage() {
	fmt.Print(
		"Usage of serverctl:\n" +
			"    serverctl -p [RPC port] [command] [args...]\n")
	fmt.Println()
	fmt.Println("Flags:")
	pflag.CommandLine.PrintDefaults()
	fmt.Println()
	fmt.Println("Available commands:")
	for name, cmd := range commands {
		fmt.Printf("    %v: %v.\n", name, cmd.description)
	}
}

var lvlToString = map[logger.LogLevel]string{
	logger.LevelTrace:   "trace",
	logger.LevelDebug:   "debug",
	logger.LevelInfo:    "info",
	logger.LevelWarning: "warn",
	logger.LevelError:   "error",
	logger.LevelFatal:   "fatal",
}

func logFormat(msg string, lvl logger.LogLevel) string {
	return fmt.Sprintf("%v: %v\n", lvlToString[lvl], msg)
}
