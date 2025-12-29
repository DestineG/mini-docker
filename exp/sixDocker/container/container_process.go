// exp/sixDocker/container/container_process.go

package container

import (
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"

	log "github.com/sirupsen/logrus"
)

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
	if err := os.Mkdir(writerURL, 0777); err != nil {
		log.Errorf("Mkdir dir %s error. %v", writerURL, err)
	}
}

func CreateMountPoint(rootUrl string, mntUrl string) {
	if err := os.Mkdir(mntUrl, 0777); err != nil {
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
	for _, v := range volumes {
		parts := strings.Split(v, ":")
		if len(parts) < 2 {
			continue
		}
		target := parts[1]
		DeleteMountPointOfVolume(mntUrl, target)
	}
	cmd := exec.Command("umount", mntUrl)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Errorf("Umount %s error: %v", mntUrl, err)
	}
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
		log.Errorf("Umount volume dir %s error: %v", containerURL, err)
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
