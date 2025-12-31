// exp/sixDocker/run.go

package main

import (
	"fmt"
	"os"
	"path"
	"sixDocker/cgroups"
	"sixDocker/cgroups/subsystems"
	"sixDocker/container"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

func Run(resConf *subsystems.ResourceConfig, tty bool, volume []string, containerName string, envSlice []string, command []string) {
	// 准备容器的根进程 使用当前可执行文件 + init 进行启动
	// 返回父进程对象和用于和子进程通信的管道
	parent, writePipe := container.NewParentProcess()
	if parent == nil {
		log.Errorf("New parent process error")
		return
	}

	if tty {
		parent.Stdin = os.Stdin
		parent.Stdout = os.Stdout
		parent.Stderr = os.Stderr
	}

	// 设置环境变量
	parent.Env = append(os.Environ(), envSlice...)

	// 生成容器 config，pid 只能在 Start 之后才能获取到，因此先用 -1 占位
	containerInfo, err := container.CreateContainerInfoByName(containerName, -1, command, volume)
	if err != nil {
		log.Errorf("Create container info error: %v", err)
		return
	}

	// ufs 创建
	mntURL, err := container.NewWorkSpace(containerInfo.Name, "busybox", volume)
	if err != nil {
		log.Errorf("New workspace error: %v", err)
		return
	}
	// 设置父进程的根文件系统，init 进程会将 Dir 作为自己的根文件系统
	parent.Dir = mntURL

	// logs 实现
	// 对parent的操作需要在start之前完成，因为start之后parent的某些属性会被锁定
	if !tty {
		logFileDir := fmt.Sprintf(container.DefaultInfoLocation, containerInfo.Name)
		// 先创建目录
		if err := os.MkdirAll(logFileDir, 0755); err != nil {
			log.Errorf("MkdirAll log file dir %s error: %v", logFileDir, err)
			return
		}
		logFilePath := path.Join(logFileDir, container.ContainerLogFile)
		logFile, err := os.Create(logFilePath)
		if err != nil {
			log.Errorf("Create log file %s error: %v", logFilePath, err)
			return
		}
		parent.Stdout = logFile
		parent.Stderr = logFile
		parent.Stdin = nil
	}

	// 启动子进程 ./sixDocker
	// 子进程会在 readUserCommand 中阻塞等待父进程通过管道发送命令(sendInitCommand(command, writePipe))
	if err := parent.Start(); err != nil {
		log.Error(err)
		return
	}

	// 记录容器Pid（必须在Start()之后，因为 Process.Pid 只有在Start()后才可用）
	log.Infof("Container %s PID %d", containerInfo.Name, parent.Process.Pid)
	containerInfo.Pid = strconv.Itoa(parent.Process.Pid)
	if err := container.UpdateContainerInfoByName(containerInfo.Name, containerInfo); err != nil {
		log.Errorf("Update container info error: %v", err)
		return
	}

	// 创建cgroup管理器
	cgroupManager := cgroups.NewCgroupManager("sixDocker-cgroup")
	// 在非tty模式下，父进程不会等待子进程结束，在父进程执行回收时，子进程可能还没有退出，导致 cgroupManager 无法回收
	// 因此在非tty模式下，不进行资源回收
	if tty {
		defer cgroupManager.Destroy()
	}

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
		// 删除容器
		if err := container.DeleteContainer(containerInfo.Name, true); err != nil {
			log.Errorf("Delete container error: %v", err)
		}
		os.Exit(0)
	}
}

func sendInitCommand(comArray []string, writePipe *os.File) {
	command := strings.Join(comArray, " ")
	log.Infof("command all is %s", command)
	writePipe.WriteString(command)
	writePipe.Close()
}
