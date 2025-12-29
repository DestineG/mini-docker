// exp/sixDocker/container/container_process.go

package container

import (
	"os"
	"os/exec"
	"syscall"

	log "github.com/sirupsen/logrus"
)

func NewParentProcess(tty bool) (*exec.Cmd, *os.File) {
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
	cmd.Dir = "/root/busybox"
	return cmd, writePipe
}

func NewPipe() (*os.File, *os.File, error) {
	read, write, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	return read, write, nil
}
