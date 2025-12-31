// exp/sixDocker/container/container_process.go

package container

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/olekukonko/tablewriter"

	log "github.com/sirupsen/logrus"
)

var (
	RUNNING             string = "running"
	STOPPED             string = "stopped"
	EXIT                string = "exited"
	DefaultInfoLocation string = "/var/run/sixDocker/%s/"
	ConfigName          string = "config.json"
	ContainerLogFile    string = "container.log"
)

type ContainerInfo struct {
	Pid         string   `json:"pid"`         // 容器init进程的pid
	Id          string   `json:"id"`          // 容器id
	Name        string   `json:"name"`        // 容器名称
	Command     string   `json:"command"`     // 容器内init进程要执行的命令
	CreatedTime string   `json:"createdTime"` // 容器创建时间
	Status      string   `json:"status"`      // 容器状态
	Volume      []string `json:"volume"`      // 容器挂载的卷
}

func NewParentProcess(tty bool, volume []string) (*exec.Cmd, *os.File) {
	readPipe, writePipe, err := NewPipe()
	if err != nil {
		log.Errorf("New pipe error %v", err)
		return nil, nil
	}
	cmd := exec.Command("/proc/self/exe", "init")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS |
			syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC,
	}
	if tty {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	// UNIX会在子进程启动前会将ExtraFiles中的文件描述符从3开始依次往后分配，也就是说描述符是属于父进程
	// 启动子进程后 子进程会继承父进程的文件描述符表
	// 因此在init进程中 通过3号文件描述符就可以获取到管道的读端
	cmd.ExtraFiles = []*os.File{readPipe}
	// 指定容器进程的工作目录，/root/busybox 存放的是容器的根文件系统
	rootURL := "/workspace/projects/go/dockerDev/unionfs/aufs/busybox"
	mntURL := path.Join(rootURL, "mnt")
	NewWorkSpace(rootURL, mntURL, volume)
	cmd.Dir = mntURL
	return cmd, writePipe
}

func NewPipe() (*os.File, *os.File, error) {
	read, write, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return read, write, nil
}

func NewWorkSpace(rootUrl string, mntUrl string, volumes []string) {
	CreateReadOnlyLayer(rootUrl)
	CreateWriterLayer(rootUrl)
	CreateMountPoint(rootUrl, mntUrl)

	for _, v := range volumes {
		parts := strings.Split(v, ":")

		if len(parts) < 2 {
			log.Errorf("Invalid volume format: %s", v)
			continue
		}

		source := parts[0]
		target := parts[1]

		// 默认 rw
		mode := "rw"
		if len(parts) == 3 {
			mode = parts[2]
		}

		CreateVolume(source, target, mntUrl, mode)
	}
}

func CreateReadOnlyLayer(rootUrl string) {
	busyboxURL := path.Join(rootUrl, "busybox")
	busyboxTarURL := path.Join(rootUrl, "busybox.tar")
	if _, err := os.Stat(busyboxURL); err != nil {
		log.Infof("Fail to judge whether dir %s exists. %v", busyboxURL, err)
		// 目录不存在 则创建该目录
		if os.IsNotExist(err) {
			if err := os.Mkdir(busyboxURL, 0777); err != nil {
				log.Errorf("Mkdir dir %s error. %v", busyboxURL, err)
			}
			// 解压 busybox.tar 到 busybox 目录下
			if err := exec.Command("tar", "-xvf", busyboxTarURL, "-C", busyboxURL).Run(); err != nil {
				log.Errorf("Tar busybox tar %s error. %v", busyboxTarURL, err)
			}
		}
	}
}

func CreateWriterLayer(rootUrl string) {
	writerURL := path.Join(rootUrl, "writeLayer")
	if err := os.MkdirAll(writerURL, 0777); err != nil {
		log.Errorf("Mkdir dir %s error. %v", writerURL, err)
	}
}

func CreateMountPoint(rootUrl string, mntUrl string) {
	if err := os.MkdirAll(mntUrl, 0777); err != nil {
		log.Errorf("Mkdir dir %s error. %v", mntUrl, err)
	}
	dirs := "dirs=" + path.Join(rootUrl, "writeLayer") + ":" + path.Join(rootUrl, "busybox")
	cmd := exec.Command("mount", "-t", "aufs", "-o", dirs, "none", mntUrl)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Errorf("Mount aufs error: %v", err)
	}
}

func DeleteWorkSpace(rootUrl string, mntUrl string, volumes []string) {
	DeleteMountPoint(mntUrl, volumes)
	DeleteWriteLayer(rootUrl)
}

func DeleteMountPoint(mntUrl string, volumes []string) {
	// 先卸载所有卷挂载点
	for _, v := range volumes {
		parts := strings.Split(v, ":")
		if len(parts) < 2 {
			continue
		}
		target := parts[1]
		DeleteMountPointOfVolume(mntUrl, target)
	}

	// 卸载主挂载点，如果失败则尝试 lazy unmount
	cmd := exec.Command("umount", mntUrl)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Warnf("Umount %s error, trying lazy unmount: %v", mntUrl, err)
		// 尝试 lazy unmount
		cmd = exec.Command("umount", "-l", mntUrl)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Errorf("Lazy umount %s error: %v", mntUrl, err)
			return // 如果卸载失败，不删除目录
		}
	}

	// 等待一下确保卸载完成
	time.Sleep(100 * time.Millisecond)

	// 删除目录
	if err := os.RemoveAll(mntUrl); err != nil {
		log.Errorf("Remove dir %s error: %v", mntUrl, err)
	}
}

func DeleteWriteLayer(rootUrl string) {
	writerURL := path.Join(rootUrl, "writeLayer")
	if err := os.RemoveAll(writerURL); err != nil {
		log.Errorf("Remove dir %s error: %v", writerURL, err)
	}
}

func CreateVolume(source string, target string, mntUrl string, mode string) {
	// 宿主机目录不存在则创建
	if _, err := os.Stat(source); err != nil {
		if os.IsNotExist(err) {
			log.Warnf("Source volume %s does not exist. Creating it.", source)
			if err := os.MkdirAll(source, 0777); err != nil {
				log.Errorf("Mkdir volume source dir %s error. %v", source, err)
			}
		}
	}

	// 容器内目录不存在则创建
	containerURL := path.Join(mntUrl, target)
	if err := os.MkdirAll(containerURL, 0777); err != nil {
		log.Errorf("Mkdir volume dir %s error. %v", containerURL, err)
	}

	// 挂载宿主机目录到容器内目录
	cmd := exec.Command("mount", "--bind", source, containerURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Errorf("Mount volume %s to %s error: %v", source, containerURL, err)
		return
	}

	// 如果是只读模式，则重新以只读方式挂载一次
	if mode == "ro" {
		cmd = exec.Command("mount", "-o", "remount,ro,bind", source, containerURL)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Errorf("Remount volume %s to %s error: %v", source, containerURL, err)
		}
	}
}

func DeleteMountPointOfVolume(mntUrl string, target string) {
	containerURL := path.Join(mntUrl, target)
	cmd := exec.Command("umount", containerURL)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Warnf("Umount volume dir %s error, trying lazy unmount: %v", containerURL, err)
		// 尝试 lazy unmount
		cmd = exec.Command("umount", "-l", containerURL)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Errorf("Lazy umount volume dir %s error: %v", containerURL, err)
		}
	}
}

// 目前仅支持保存文件系统目录为 /workspace/projects/go/dockerDev/exp/unionfs/aufs/mnt 的容器
func CommitContainer(imageName string) {
	rootUrl := "/workspace/projects/go/dockerDev/unionfs/aufs/busybox"
	mntURL := path.Join(rootUrl, "mnt")
	imageURL := path.Join(rootUrl, imageName+".tar")
	cmd := exec.Command("tar", "-cvf", imageURL, "-C", mntURL, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Commit container failed: %v, output: %s", err, string(output))
	}
}

// 打印正在运行的容器信息
func ListContainers() {
	dirURL := fmt.Sprintf(DefaultInfoLocation, "")
	dirURL = dirURL[:len(dirURL)-1]
	files, err := ioutil.ReadDir(dirURL)
	if err != nil {
		log.Errorf("Read dir %s error: %v", dirURL, err)
		return
	}
	var containers []ContainerInfo
	for _, file := range files {
		containerDir := path.Join(dirURL, file.Name())
		configFilePath := path.Join(containerDir, ConfigName)
		content, err := ioutil.ReadFile(configFilePath)
		if err != nil {
			log.Errorf("Read file %s error: %v", configFilePath, err)
			continue
		}
		var containerInfo ContainerInfo
		if err := json.Unmarshal(content, &containerInfo); err != nil {
			log.Errorf("Unmarshal container info error: %v", err)
			continue
		}
		containers = append(containers, containerInfo)
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Name", "PID", "Command", "CreatedTime", "Status"})

	for _, container := range containers {
		table.Append([]string{
			container.Id,
			container.Name,
			container.Pid,
			container.Command,
			container.CreatedTime,
			container.Status,
		})
	}
	table.Render()
}

func LogContainer(containerId string) {
	containerDir := fmt.Sprintf(DefaultInfoLocation, containerId)
	logFilePath := path.Join(containerDir, ContainerLogFile)
	content, err := ioutil.ReadFile(logFilePath)
	if err != nil {
		log.Errorf("Read log file %s error: %v", logFilePath, err)
		return
	}
	fmt.Fprint(os.Stdout, string(content))
}

func StopContainer(containerId string) error {
	containerDir := fmt.Sprintf(DefaultInfoLocation, containerId)
	configFilePath := path.Join(containerDir, ConfigName)
	content, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		log.Errorf("Read file %s error: %v", configFilePath, err)
		return err
	}
	var containerInfo ContainerInfo
	if err := json.Unmarshal(content, &containerInfo); err != nil {
		log.Errorf("Unmarshal container info error: %v", err)
		return err
	}

	pid := containerInfo.Pid
	cmd := exec.Command("kill", "-9", pid)
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Errorf("Kill container %s failed: %v, output: %s", containerId, err, string(output))
		return err
	}

	// 更新容器状态为 stopped
	containerInfo.Status = STOPPED
	updatedContent, err := json.Marshal(containerInfo)
	if err != nil {
		log.Errorf("Marshal updated container info error: %v", err)
		return err
	}
	if err := os.WriteFile(configFilePath, updatedContent, 0622); err != nil {
		log.Errorf("Write updated container info to file %s error: %v", configFilePath, err)
		return err
	}
	return nil
}

func RemoveContainer(containerId string) error {
	containerDir := fmt.Sprintf(DefaultInfoLocation, containerId)
	containerConfigPath := path.Join(containerDir, ConfigName)
	content, err := ioutil.ReadFile(containerConfigPath)
	if err != nil {
		log.Errorf("Read file %s error: %v", containerConfigPath, err)
		return err
	}
	var containerInfo ContainerInfo
	if err := json.Unmarshal(content, &containerInfo); err != nil {
		log.Errorf("Unmarshal container info error: %v", err)
		return err
	}
	if containerInfo.Status == RUNNING {
		return fmt.Errorf("cannot remove a running container, please stop it first")
	}
	log.Infof("Removing container %s", containerId)
	if err := os.RemoveAll(containerDir); err != nil {
		log.Errorf("Remove container dir %s error: %v", containerDir, err)
		return err
	}
	return nil
}
