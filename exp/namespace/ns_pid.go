// exp/ns_pid.go

package main

import (
	"os/exec"
	"syscall"
	"os"
	"log"
)

func main() {
	cmd := exec.Command("sh")
	// 子进程参数配置
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID,
	}

	// 开启终端输入
	cmd.Stdin = os.Stdin
	// 开启终端输出
	cmd.Stdout = os.Stdout
	// 开启终端错误输出
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Fatalf("cmd.Run() failed with %s\n", err)
	}
}