// exp/sixDocker/run.go

package main

import (
	"os"
	"path"
	"sixDocker/cgroups"
	"sixDocker/cgroups/subsystems"
	"sixDocker/container"
	"strings"

	log "github.com/sirupsen/logrus"
)

func Run(tty bool, resConf *subsystems.ResourceConfig, volume []string, command []string) {
	// 准备容器的根进程 使用当前可执行文件 + init 进行启动
	// 返回父进程对象和用于和子进程通信的管道
	parent, writePipe := container.NewParentProcess(tty, volume)
	if parent == nil {
		log.Errorf("New parent process error")
		return
	}

	// 启动子进程 ./sixDocker
	// 子进程会在 readUserCommand 中阻塞等待父进程通过管道发送命令(sendInitCommand(command, writePipe))
	if err := parent.Start(); err != nil {
		log.Error(err)
	}

	// 创建cgroup管理器
	cgroupManager := cgroups.NewCgroupManager("sixDocker-cgroup")
	defer cgroupManager.Destroy()

	// 设置资源限制
	if resConf != nil {
		if err := cgroupManager.Set(resConf); err != nil {
			log.Errorf("Set cgroup error: %v", err)
			return
		}
	}
	// 将容器进程加入到各个subsystem挂载对应的cgroup中
	if err := cgroupManager.Apply(parent.Process.Pid); err != nil {
		log.Errorf("Apply cgroup error: %v", err)
		return
	}
	// 通过管道传递初始化容器进程要执行的命令
	log.Infof("parent writePipe %v", writePipe)
	// 子进程接收到数据后会从管道中读取命令并执行
	sendInitCommand(command, writePipe)

	// 此处是为了实现 -d 参数的功能
	// -d 为真的话就不会回收资源，在当前条件下可能会导致 ufs/mnt 目录无法卸载
	// 因为父进程退出后 子进程会被init进程收养 导致无法回收
	// 这里简单处理为 如果是tty模式下才等待容器进程结束 并回收资源
	if tty {
		// 等待容器进程结束
		parent.Wait()
		// 容器进程退出后 清理资源
		// 此处的 rootURL 和 mntURL 要和 NewParentProcess 中的一致
		log.Infof("container %d exited", parent.Process.Pid)
		rootURL := "/workspace/projects/go/dockerDev/unionfs/aufs/busybox"
		mntURL := path.Join(rootURL, "mnt")
		container.DeleteWorkSpace(rootURL, mntURL, volume)
		os.Exit(0)
	}
}

func sendInitCommand(comArray []string, writePipe *os.File) {
	command := strings.Join(comArray, " ")
	log.Infof("command all is %s", command)
	writePipe.WriteString(command)
	writePipe.Close()
}
