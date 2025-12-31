// exp/sixDocker/main_command.go

package main

import (
	"fmt"
	"os"
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
		cli.StringFlag{
			Name:  "name",
			Usage: "container name",
		},
		cli.StringSliceFlag{
			Name:  "e",
			Usage: "environment variables",
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
		containerName := context.String("name")
		// 从cli上下文中获取资源限制参数
		resConf := &subsystems.ResourceConfig{
			CpuShare:    context.String("cpushare"),
			CpuSet:      context.String("cpuset"),
			MemoryLimit: context.String("m"),
		}
		// 获取 环境变量
		envSlice := context.StringSlice("e")
		// 获取 挂载卷
		volume := context.StringSlice("v")
		Run(resConf, tty, volume, containerName, envSlice, cmdArray)
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
			Name:  "n",
			Usage: "The name or ID of the container to commit",
		},
		cli.StringFlag{
			Name:  "t",
			Usage: "The name of the new image",
		},
	},
	Action: func(context *cli.Context) error {
		containerName := context.String("n")
		imageName := context.String("t")
		log.Infof("commit: Container name: %s, Image name: %s", containerName, imageName)
		return container.CommitContainer(containerName, imageName)
	},
}

var listCommand = cli.Command{
	Name:  "ps",
	Usage: "List all the containers",
	Action: func(context *cli.Context) error {
		container.ListContainers()
		return nil
	},
}

var logsCommand = cli.Command{
	Name: "logs",
	Usage: `Print logs of a container
			mydocker logs [containerName]`,
	Action: func(context *cli.Context) error {
		if len(context.Args()) < 1 {
			return fmt.Errorf("missing container name")
		}

		containerName := context.Args().Get(0)
		container.LogContainer(containerName)
		return nil
	},
}

var execCommand = cli.Command{
	Name: "exec",
	Usage: `Exec a command into existing container
			mydocker exec [containerName] [command]`,
	Action: func(context *cli.Context) error {
		// 如果有环境变量 ENV_EXEC_PID ，说明会被 CGO 拦截就肯定不会走到这里
		// 此处只是做一个防御
		if os.Getenv(container.ENV_EXEC_PID) != "" {
			return nil
		}

		if len(context.Args()) < 2 {
			return fmt.Errorf("missing container name or command")
		}

		containerName := context.Args().Get(0)
		var cmdArray []string
		for _, arg := range context.Args().Tail() {
			cmdArray = append(cmdArray, arg)
		}

		return container.ExecContainer(containerName, cmdArray)
	},
}

var stopCpmmand = cli.Command{
	Name: "stop",
	Usage: `Stop a container
			mydocker stop [containerName]`,
	Action: func(context *cli.Context) error {
		if len(context.Args()) < 1 {
			return fmt.Errorf("missing container containerName")
		}

		containerName := context.Args().Get(0)
		return container.StopContainer(containerName)
	},
}

var removeCommand = cli.Command{
	Name: "rm",
	Usage: `Remove a container
			mydocker rm [containerName]`,
	Action: func(context *cli.Context) error {
		if len(context.Args()) < 1 {
			return fmt.Errorf("missing container name")
		}

		containerName := context.Args().Get(0)
		return container.DeleteContainer(containerName, false)
	},
}

var ShowAllImagesCommand = cli.Command{
	Name:  "images",
	Usage: "List all the images",
	Action: func(context *cli.Context) error {
		return container.ShowAllImages()
	},
}
