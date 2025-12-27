// exp/cgroups/cgroup.go

package main

import (
	"os"
	"os/exec"
	"path"
	"fmt"
	"io/ioutil"
	"syscall"
	"strconv"
)
const cgroupMemoryHierarchyMount = "/sys/fs/cgroup/memory/"

func main() {
	// 如果是当前进程的子进程，那就运行压力测试程序
	fmt.Printf("os.Args[0]: %s\n", os.Args[0])
	if os.Args[0] == "/proc/self/exe" {
		fmt.Printf("current pid %d\n", os.Getpid())
		cmd := exec.Command("sh", "-c", "stress --vm-bytes 200m --vm-keep -m 1")
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		// Run: 阻塞执行
		if err := cmd.Run(); err != nil {
			fmt.Printf("cmd.Run() failed with %s\n", err)
			os.Exit(1)
		}
	}
	// 父进程运行这里的代码
	// 重入当前可执行文件 "/proc/self/exe" 会作为子进程的第1个参数
	cmd := exec.Command("/proc/self/exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS,
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// 异步执行
	if err := cmd.Start(); err != nil {
		fmt.Printf("cmd.Start() failed with %s\n", err)
		os.Exit(1)
	} else {
		fmt.Printf("%v\n", cmd.Process.Pid)
		os.Mkdir(path.Join(cgroupMemoryHierarchyMount, "testmemorylimit"), 0755)
		ioutil.WriteFile(path.Join(cgroupMemoryHierarchyMount, "testmemorylimit", "tasks"), []byte(strconv.Itoa(cmd.Process.Pid)), 0644)
		ioutil.WriteFile(path.Join(cgroupMemoryHierarchyMount, "testmemorylimit", "memory.limit_in_bytes"), []byte("100000000"), 0644)
	}
	// 起开
	cmd.Process.Wait()
}