// exp/sixDocker/container/init.go

package container

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"

	log "github.com/sirupsen/logrus"
)

func RunContainerInitProcess() error {
	// 从父进程的第4个文件描述符中读取管道传递过来的命令
	cmdArray := readUserCommand()
	if len(cmdArray) == 0 {
		return fmt.Errorf("Run container get user command error, cmdArray is nil")
	}

	// 设置根文件系统和挂载proc文件系统
	// 后续 exec.LookPath 会在新的根文件系统中查找可执行文件
	setUpMount()

	// 从当前进程看到的环境变量中查找可执行文件的路径
	path, err := exec.LookPath(cmdArray[0])
	if err != nil {
		log.Errorf("Exec loop path error %v", err)
		return err
	}
	log.Infof("Find path %s", path)
	if err := syscall.Exec(path, cmdArray[0:], os.Environ()); err != nil {
		log.Errorf(err.Error())
	}
	return nil
}

func readUserCommand() []string {
	// 在NewParentProcess中将pipe的读端设置为3号文件描述符
	// 在init进程中通过3号文件描述符获取管道读端, 设置pipe的Name为pipe
	pipe := os.NewFile(uintptr(3), "pipe")
	// 等待父进程通过管道写入命令(阻塞) 也就是sendInitCommand函数的执行
	msg, err := ioutil.ReadAll(pipe)
	if err != nil {
		log.Errorf("init read pipe error %v", err)
		return nil
	}
	msgStr := string(msg)
	return strings.Split(msgStr, " ")
}

// 将当前工作目录作为新的根文件系统 并挂载proc文件系统
func setUpMount() {
	pwd, err := os.Getwd()
	if err != nil {
		log.Errorf("Get current location error %v", err)
		return
	}
	log.Infof("Current location is %s", pwd)
	if err := pivotRoot(pwd); err != nil {
		log.Errorf("pivot root error %v", err)
		return
	}

	// proc 文件系统和 namespace 密切相关 每个 namespace 都有自己独立的 proc 文件系统
	// 切换新的根文件系统后需要在它上面挂载 proc 文件系统
	// 挂载 proc 文件系统
	defaultMountFlags := syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV
	if err := syscall.Mount("proc", "/proc", "proc", uintptr(defaultMountFlags), ""); err != nil {
		log.Errorf("Mount proc file system error: %v", err)
		return
	}

	// 为新 rootfs 提供可用的 /dev：
	// 挂载 tmpfs 以存放容器所需的最小设备节点(如 /dev/null、/dev/tty)
	if err := syscall.Mount("tmpfs", "/dev", "tmpfs", syscall.MS_NOSUID|syscall.MS_STRICTATIME, "mode=755"); err != nil {
		log.Errorf("Mount tmpfs to /dev error: %v", err)
		return
	}
}

func pivotRoot(root string) error {
	// PivotRoot(newroot, putold)要求newroot和putold必须不在同一个文件系统中
	// 因此需要先将newroot进行bind mount，bind mount可以将一个目录单独作为一个文件系统挂载
	if err := syscall.Mount(root, root, "bind", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		return fmt.Errorf("Mount rootfs to itself error: %v", err)
	}
	// 创建putold目录
	pivotDir := path.Join(root, ".pivot_root")
	if err := os.Mkdir(pivotDir, 0777); err != nil {
		return fmt.Errorf("Create pivot_root dir error: %v", err)
	}

	// 调用pivot_root系统调用
	if err := syscall.PivotRoot(root, pivotDir); err != nil {
		return fmt.Errorf("Pivot rootfs error: %v", err)
	}

	// 修改当前工作目录到根目录
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("Chdir / error: %v", err)
	}

	// 获取新文件系统目录下的.pivot_root目录的绝对路径 并卸载它
	pivotDir = path.Join("/", ".pivot_root")
	if err := syscall.Unmount(pivotDir, syscall.MNT_DETACH); err != nil {
		return fmt.Errorf("Unmount pivot_root dir error: %v", err)
	}

	// 删除.pivot_root目录
	return os.Remove(pivotDir)
}
