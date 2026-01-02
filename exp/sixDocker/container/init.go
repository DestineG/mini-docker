// exp/sixDocker/container/init.go

package container

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
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

// 在NewParentProcess中将pipe的读端设置为3号文件描述符
// 在init进程中通过3号文件描述符获取管道读端, 设置pipe的Name为pipe
// 等待父进程通过管道写入命令(阻塞) 也就是sendInitCommand函数的执行
func readUserCommand() []string {
	pipe := os.NewFile(uintptr(3), "pipe")
	msg, err := ioutil.ReadAll(pipe)
	if err != nil {
		log.Errorf("init read pipe error %v", err)
		return nil
	}

	var cmdArray []string
	// 将读取到的 JSON 数据还原为 []string 切片
	if err := json.Unmarshal(msg, &cmdArray); err != nil {
		log.Errorf("Unmarshal command error: %v", err)
		return nil
	}
	return cmdArray
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

	// 挂载 proc
	defaultMountFlags := syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV
	if err := syscall.Mount("proc", "/proc", "proc", uintptr(defaultMountFlags), ""); err != nil {
		log.Errorf("Mount proc error: %v", err)
		return
	}

	// 挂载 tmpfs 到 /dev
	if err := syscall.Mount("tmpfs", "/dev", "tmpfs", syscall.MS_NOSUID|syscall.MS_STRICTATIME, "mode=755"); err != nil {
		log.Errorf("Mount tmpfs to /dev error: %v", err)
		return
	}

	// 关键：手动创建设备节点
	// 必须要手动创建 /dev/null，否则 Nginx 等程序无法启动
	// 参数说明: 路径, 权限|字符设备类型, 设备号(通过 makedev 计算)
	// /dev/null 的主设备号是 1, 次设备号是 3
	if err := syscall.Mknod("/dev/null", 0666|syscall.S_IFCHR, int(makedev(1, 3))); err != nil {
		log.Errorf("Mknod /dev/null error: %v", err)
	}

	// 建议顺便把 /dev/zero 也建了，很多程序需要它 (1, 5)
	if err := syscall.Mknod("/dev/zero", 0666|syscall.S_IFCHR, int(makedev(1, 5))); err != nil {
		log.Errorf("Mknod /dev/zero error: %v", err)
	}
}

// 辅助函数：计算 Linux 设备号
func makedev(major, minor uint32) uint64 {
	// 先转 uint64，再位移
	return uint64(minor&0xff) |
		uint64((major&0xfff)<<8) |
		uint64(minor&^0xff)<<12 |
		uint64(major&^0xfff)<<32
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
