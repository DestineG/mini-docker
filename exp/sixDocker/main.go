// exp/sixDocker/main.go

package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const usage = `sixDocker is a simple container runtime implementation demo.
	The purpose of this project is to learn how docker works and how to write a container runtime by myself.
	Enjoy it, just for fun!`

func main() {
	// cli程序
	app := cli.NewApp()
	app.Name = "sixDocker"
	app.Usage = usage

	// cli子命令定义
	app.Commands = []cli.Command{
		initCommand,
		runCommand,
		commitCommand,
		listCommand,
		logsCommand,
		execCommand,
	}

	// cli全局配置
	app.Before = func(context *cli.Context) error {
		// Log as JSON instead of the default ASCII formatter.
		log.SetFormatter(&log.TextFormatter{})
		log.SetOutput(os.Stdout)
		return nil
	}

	// cli运行 解析命令行参数 ./sixDocker run -ti -m 100m -- stress --vm-bytes 800m --vm-keep -m 1
	log.Infof("main - os.Args: %v", os.Args)
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
