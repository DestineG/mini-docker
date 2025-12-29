// exp/sixDocker/main_command.go

package main

import (
	"fmt"
	"sixDocker/cgroups/subsystems"
	"sixDocker/container"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var runCommand = cli.Command{
	Name: "run",
	Usage: `Create a container with namespace and cgroup limit
			mydocker run -it [command]`,
	// cli选项定义 选项参数以 - 或者 -- 开头
	// boolFlag: 不出现则为false 出现则为true
	// stringFlag: 不出现则为默认值 出现则为其后跟的字符串值
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "ti",
			Usage: "enable tty",
		},
		cli.StringFlag{
			Name:  "m",
			Usage: "memory limit",
		},
		cli.StringFlag{
			Name:  "cpushare",
			Usage: "cpushare limit",
		},
		cli.StringFlag{
			Name:  "cpuset",
			Usage: "cpuset limit",
		},
		cli.StringSliceFlag{
			Name:  "v",
			Usage: "volume",
		},
		cli.BoolFlag{
			Name:  "d",
			Usage: "run container in background",
		},
	},
	Action: func(context *cli.Context) error {
		if len(context.Args()) < 1 {
			return fmt.Errorf("missing container command")
		}

		// 获取未被flag解析的参数(命令和命令参数)
		args := context.Args()
		if len(args) == 0 {
			return fmt.Errorf("missing container command")
		}
		var cmdArray []string
		for _, arg := range context.Args() {
			cmdArray = append(cmdArray, arg)
		}
		tty := context.Bool("ti")
		d := context.Bool("d")
		if tty && d {
			return fmt.Errorf("ti and d paramter can not both provided")
		}
		// 从cli上下文中获取资源限制参数
		resConf := &subsystems.ResourceConfig{
			CpuShare:    context.String("cpushare"),
			CpuSet:      context.String("cpuset"),
			MemoryLimit: context.String("m"),
		}
		volume := context.StringSlice("v")
		Run(tty, resConf, volume, cmdArray)
		return nil
	},
}

var initCommand = cli.Command{
	Name:  "init",
	Usage: "Init container process run user's process in container. Do not call it outside",
	Action: func(context *cli.Context) error {
		log.Infof("init come on")
		err := container.RunContainerInitProcess()
		return err
	},
}

var commitCommand = cli.Command{
	Name:  "commit",
	Usage: "Commit a container into image",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "imageName",
			Usage: "The name or ID of the container to commit",
		},
	},
	Action: func(context *cli.Context) error {
		imageName := context.String("imageName")
		log.Infof("commit: Image name: %s", imageName)
		container.CommitContainer(imageName)
		return nil
	},
}
