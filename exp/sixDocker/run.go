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

	// 先生成容器ID（不依赖PID），用于日志文件路径
	containerId := randStringBytes(10)
	if containerName == "" {
		containerName = containerId
	}

	// logs 实现
	// 对parent的操作需要在start之前完成，因为start之后parent的某些属性会被锁定
	if !tty {
		logFileDir := fmt.Sprintf(container.DefaultInfoLocation, containerId)
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

	// 记录容器信息（必须在Start()之后，因为 Process.Pid 只有在Start()后才可用）
	if err := recordContainerInfoWithId(containerId, parent.Process.Pid, command, containerName); err != nil {
		log.Errorf("Record container info error: %v", err)
		return
	}
	log.Infof("Container Id: %s", containerId)

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
	err := recordContainerInfoWithId(id, containerPID, commandArray, containerName)
	return id, err
}

func recordContainerInfoWithId(containerId string, containerPID int, commandArray []string, containerName string) error {
	CreatedTime := time.Now().Format("2006-01-02 15:04:05")
	command := strings.Join(commandArray, "")
	if containerName == "" {
		containerName = containerId
	}
	containerInfo := &container.ContainerInfo{
		Pid:         strconv.Itoa(containerPID),
		Id:          containerId,
		Command:     command,
		CreatedTime: CreatedTime,
		Status:      container.RUNNING,
		Name:        containerName,
	}
	jsonBytes, err := json.Marshal(containerInfo)
	if err != nil {
		log.Errorf("Record container info error: %v", err)
		return err
	}
	dirUrl := fmt.Sprintf(container.DefaultInfoLocation, containerId)
	if err := os.MkdirAll(dirUrl, 0622); err != nil {
		log.Errorf("MkdirAll %s error: %v", dirUrl, err)
		return err
	}
	fileName := path.Join(dirUrl, container.ConfigName)
	if err := os.WriteFile(fileName, jsonBytes, 0622); err != nil {
		log.Errorf("Write file %s error: %v", fileName, err)
		return err
	}
	return nil
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
