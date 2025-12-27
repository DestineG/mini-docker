// exp/sixDocker/run.go

package main

import (
	"os"
	"sixDocker/container"

	log "github.com/sirupsen/logrus"
)

func Run(tty bool, command string) {
	// 准备容器的根进程 使用当前可执行文件 + init 子命令进行启动
	parent := container.NewParentProcess(tty, command)
	// 启动根进程 ./sixDocker init [command]
	if err := parent.Start(); err != nil {
		log.Error(err)
	}
	parent.Wait()
	os.Exit(-1)
}
