// exp/sixDocker/run.go

package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path"
	"sixDocker/cgroups"
	"sixDocker/cgroups/subsystems"
	"sixDocker/container"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

func Run(tty bool, resConf *subsystems.ResourceConfig, volume []string, containerName string, command []string) {
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

	// 记录容器信息
	containerId, err := recordContainerInfo(parent.Process.Pid, command, containerName)
	if err != nil {
		log.Errorf("Record container info error: %v", err)
		return
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
		// 删除容器信息
		deleteContainerInfo(containerId)
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

func recordContainerInfo(containerPID int, commandArray []string, containerName string) (string, error) {
	id := randStringBytes(10)
	CreatedTime := time.Now().Format("2006-01-02 15:04:05")
	command := strings.Join(commandArray, "")
	if containerName == "" {
		containerName = id
	}
	containerInfo := &container.ContainerInfo{
		Pid:         strconv.Itoa(containerPID),
		Id:          id,
		Command:     command,
		CreatedTime: CreatedTime,
		Status:      container.RUNNING,
		Name:        containerName,
	}
	jsonBytes, err := json.Marshal(containerInfo)
	if err != nil {
		log.Errorf("Record container info error: %v", err)
		return "", err
	}
	dirUrl := fmt.Sprintf(container.DefaultInfoLocation, id)
	if err := os.MkdirAll(dirUrl, 0622); err != nil {
		log.Errorf("MkdirAll %s error: %v", dirUrl, err)
		return "", err
	}
	fileName := path.Join(dirUrl, container.ConfigName)
	if err := os.WriteFile(fileName, jsonBytes, 0622); err != nil {
		log.Errorf("Write file %s error: %v", fileName, err)
		return "", err
	}
	return id, nil
}

func deleteContainerInfo(containerId string) {
	dirUrl := fmt.Sprintf(container.DefaultInfoLocation, containerId)
	if err := os.RemoveAll(dirUrl); err != nil {
		log.Errorf("Remove file %s error: %v", dirUrl, err)
	}
}

func randStringBytes(n int) string {
	const letterBytes = "0123456789"
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
